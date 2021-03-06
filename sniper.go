package sharpshooter

import (
	"encoding/binary"
	"errors"
	"github.com/soyum2222/sharpshooter/protocol"
	"github.com/soyum2222/sharpshooter/tool/block"
	"math"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const (
	shooting = 1 << iota
	waittimeout
)

const (
	DEFAULT_HEAD_SIZE                          = 20
	DEFAULT_INIT_SENDWIND                      = 64
	DEFAULT_INIT_RECEWIND                      = 1 << 10
	DEFAULT_INIT_PACKSIZE                      = 1000
	DEFAULT_INIT_HEALTHTICKER                  = 1
	DEFAULT_INIT_HEALTHCHECK_TIMEOUT_TRY_COUNT = 10
	DEFAULT_INIT_HANDSHACK_TIMEOUT             = 6
	DEFAULT_INIT_RTO_UNIT                      = float64(200 * time.Millisecond)
	DEFAULT_INIT_DELAY_ACK                     = float64(200 * time.Millisecond)
	DEFAULT_INIT_INTERVAL                      = 500
)

var (
	CLOSEERROR         = errors.New("the connection is closed")
	HEALTHTIMEOUTERROR = errors.New("health monitor timeout ")
	TIMEOUERROR        = errors.New("i/o timeout")
)

type Sniper struct {
	packageSize int64
	rtt         int64
	rto         int64
	rttTimeFlag int64

	totalFlow     int64 // statistics total flow used
	effectiveFlow int64 // statistics effective flow
	interval      int64
	timeAnchor    int64

	winSize        int32
	minSize        int32
	healthTryCount int32
	shootStatus    int32
	sendId         uint32
	rcvId          uint32
	sendWinId      uint32
	rttSampId      uint32

	isClose   bool
	noLeader  bool // it not has headquarters
	staSwitch bool // statistics switch
	rttHelper uint8

	sendCache []byte
	writer    func(p []byte) (n int, err error)

	//dead line
	readDeadline  time.Time
	deadline      time.Time
	writeDeadline time.Time

	aim  *net.UDPAddr
	conn *net.UDPConn

	ammoBag       []*protocol.Ammo
	rcvAmmoBag    []*protocol.Ammo
	rcvCache      []byte
	readBlock     chan struct{}
	handShakeSign chan struct{}
	writerBlocker *block.Blocker
	closeOnce     sync.Once
	closeChan     chan struct{}
	errorSign     chan struct{}

	wrap func()
	rcv  func(ammo *protocol.Ammo)

	// fec
	fec  bool
	fece *fecEncoder
	fecd *fecDecoder

	errorContainer atomic.Value
	ackCache       []uint32
	ackSendCache   []uint32

	// ack lock
	ackLock sync.Mutex
	// send lock
	mu sync.Mutex
	// receive lock
	bemu sync.Mutex
	// close lock
	clock sync.Mutex

	chanCloser chanCloser
}

func (s *Sniper) LocalAddr() net.Addr {
	return s.conn.LocalAddr()
}

func (s *Sniper) RemoteAddr() net.Addr {
	return s.aim
}

func (s *Sniper) SetDeadline(t time.Time) error {
	s.deadline = t
	return nil
}

func (s *Sniper) SetReadDeadline(t time.Time) error {
	s.readDeadline = t
	return nil
}

func (s *Sniper) SetWriteDeadline(t time.Time) error {
	s.writeDeadline = t
	return nil
}

func NewSniper(conn *net.UDPConn, aim *net.UDPAddr) *Sniper {

	sn := &Sniper{
		aim:           aim,
		conn:          conn,
		rtt:           int64(time.Millisecond * 500),
		rto:           int64(time.Second),
		rcvAmmoBag:    make([]*protocol.Ammo, DEFAULT_INIT_RECEWIND),
		handShakeSign: make(chan struct{}, 1),
		readBlock:     make(chan struct{}, 0),
		winSize:       DEFAULT_INIT_SENDWIND,
		mu:            sync.Mutex{},
		bemu:          sync.Mutex{},
		packageSize:   DEFAULT_INIT_PACKSIZE,
		writerBlocker: block.NewBlocker(),
		closeChan:     make(chan struct{}, 0),
		errorSign:     make(chan struct{}),
		sendCache:     make([]byte, 0),
		interval:      DEFAULT_INIT_INTERVAL,
		minSize:       64,
	}

	sn.rcv = sn.rcvnoml

	sn.wrap = sn.wrapnoml

	sn.writer = sn.delaySend
	return sn
}

func (s *Sniper) addTotalFlow(flow int) {
	if s.staSwitch {
		s.totalFlow += int64(flow)
	}
}

func (s *Sniper) addEffectFlow(flow int) {
	if s.staSwitch {
		s.effectiveFlow += int64(flow)
	}
}

func (s *Sniper) FlowStatistics() (total int64, effective int64) {
	return s.totalFlow, s.effectiveFlow
}

//func (s *Sniper) SetPackageSize(size int64) {
//	s.mu.Lock()
//	defer s.mu.Unlock()
//	s.packageSize = size
//}

func (s *Sniper) SetRecWin(size int64) {
	s.bemu.Lock()
	defer s.bemu.Unlock()
	s.rcvAmmoBag = make([]*protocol.Ammo, size)
}

func (s *Sniper) SetSendWin(size int32) {
	s.bemu.Lock()
	defer s.bemu.Unlock()
	s.minSize = size
	s.winSize = size
}

func (s *Sniper) SetInterval(interval int64) {
	s.interval = interval
}

func (s *Sniper) OpenStaFlow() {
	s.staSwitch = true
}

// use FEC algorithm in communication
// this will waste some of traffic ,  but when the packet is lost
// there is a certain probability that the lost packet can be recovered
func (s *Sniper) OpenFec(dataShards, parShards int) {
	s.fecd = newFecDecoder(dataShards, parShards)
	s.fece = newFecEncoder(dataShards, parShards)
	s.wrap = s.wrapfec
	s.rcv = s.rcvfec
}

func (s *Sniper) healthMonitor() {

	select {
	default:

		if s.healthTryCount < DEFAULT_INIT_HEALTHCHECK_TIMEOUT_TRY_COUNT {

			if s.healthTryCount == 0 {
				//s.timeFlag = time.Now().UnixNano()
			}

			_, _ = s.conn.WriteToUDP(protocol.Marshal(protocol.Ammo{
				Kind: protocol.HEALTHCHECK,
			}), s.aim)

			atomic.AddInt32(&s.healthTryCount, 1)

			SystemTimedSched.Put(s.healthMonitor, time.Now().Add(time.Second*DEFAULT_INIT_HEALTHTICKER))

		} else {
			// timeout
			s.clock.Lock()
			defer s.clock.Unlock()
			if s.isClose {
				return
			}

			s.errorContainer.Store(errors.New(HEALTHTIMEOUTERROR.Error()))
			s.chanCloser.closeChan(s.errorSign)

			s.writerBlocker.Close()

			if s.noLeader {
				_ = s.conn.Close()
			}

			s.isClose = true
			s.chanCloser.closeChan(s.closeChan)
			s.chanCloser.closeChan(s.readBlock)
			return
		}

	case _, _ = <-s.closeChan:
		// if come here , then close func has been executed
		return
	}
}

// send to remote
func (s *Sniper) shoot() {
	if atomic.CompareAndSwapInt32(&s.shootStatus, 0, shooting) {

		defer atomic.StoreInt32(&s.shootStatus, 0)
		select {
		case <-s.errorSign:
			return
		case <-s.closeChan:
			return
		default:
		}

		s.mu.Lock()

		s.flush()

		s.wrap()

		if len(s.ammoBag) == 0 {
			s.mu.Unlock()
			s.timeAnchor = 0
			return
		}

		now := time.Now()
		if now.Unix()-s.timeAnchor >= 0 && s.timeAnchor != 0 {
			s.zoomoutWin()
		}

		for k := range s.ammoBag {

			if s.ammoBag[k] == nil {
				continue
			}

			if k > int(s.winSize) {
				break
			}

			if s.rttHelper%16 == 0 && s.rttSampId == 0 {
				// samp
				s.rttSampId = s.ammoBag[k].Id
				s.rttTimeFlag = now.UnixNano()
			}
			s.rttHelper++

			b := protocol.Marshal(*s.ammoBag[k])

			_, err := s.conn.WriteToUDP(b, s.aim)
			if err != nil {
				select {
				case <-s.errorSign:
				default:
					s.errorContainer.Store(errors.New(err.Error()))
					s.chanCloser.closeChan(s.errorSign)
				}
			}

			s.addTotalFlow(len(b))
		}

		s.mu.Unlock()

		rto := atomic.LoadInt64(&s.rto)

		// maybe overflow
		if rto <= 0 {
			rto = int64(250 * time.Millisecond)
		}

		interval := time.Duration(math.Min(float64(rto), float64(time.Duration(s.interval)*time.Millisecond))) * time.Nanosecond
		shootTime := now.Add(interval)
		s.timeAnchor = shootTime.Unix()

		SystemTimedSched.Put(s.shoot, shootTime)
	}
}

// remove already sent packages
func (s *Sniper) flush() {

	defer func() {
		remain := int64(s.winSize)*(s.packageSize)*2 - int64(len(s.sendCache))
		if remain > 0 {
			_ = s.writerBlocker.Pass()
		}
	}()

	var index int
	if len(s.ammoBag) == 0 {
		return
	}

	var flag bool
	for i := 0; i < len(s.ammoBag); i++ {
		if s.ammoBag[i] != nil {
			index = i
			flag = true
			break
		}
	}

	if flag {
		s.ammoBag = s.ammoBag[index:]
	} else if index == 0 {
		s.ammoBag = s.ammoBag[0:0]
		atomic.StoreUint32(&s.sendWinId, atomic.LoadUint32(&s.sendId))
		return
	}

	if len(s.ammoBag) == 0 {
		atomic.AddUint32(&s.sendWinId, uint32(index))
		return
	}
	atomic.StoreUint32(&s.sendWinId, s.ammoBag[0].Id)
}

func (s *Sniper) ack(id uint32) {

	s.ackLock.Lock()

	for i := 0; i < len(s.ackCache); i++ {
		if s.ackCache[i] == id {
			s.ackLock.Unlock()
			return
		}
	}

	s.ackCache = append(s.ackCache, id)

	s.ackLock.Unlock()
	if len(s.ackCache) > int(s.packageSize/4) {
		if s.wrapACK() {
			s.ackSender()
		}
	}
}

// timed trigger send ack
func (s *Sniper) ackTimer() {

	s.wrapACK()
	s.ackSender()

	select {
	default:

	case <-s.closeChan:
		return
	}
	SystemTimedSched.Put(s.ackTimer, time.Now().Add(time.Millisecond*30))
}

func (s *Sniper) ackSender() {
	s.ackLock.Lock()

	defer s.ackLock.Unlock()

	l := len(s.ackSendCache)
	if l != 0 {

		b := make([]byte, l*4)

		for i := 0; i < len(s.ackSendCache); i++ {
			binary.BigEndian.PutUint32(b[4*i:(i+1)*4], s.ackSendCache[i])
		}

		ammo := protocol.Ammo{
			Kind: protocol.ACK,
			Body: b,
		}

		s.ackSendCache = s.ackSendCache[0:0]

		_, err := s.conn.WriteToUDP(protocol.Marshal(ammo), s.aim)
		if err != nil {
			select {
			case <-s.errorSign:
				return
			default:
				s.errorContainer.Store(errors.New(err.Error()))
			}
		}
	}
}

func (s *Sniper) wrapACK() bool {

	s.ackLock.Lock()
	defer s.ackLock.Unlock()

	if len(s.ackCache) == 0 {
		return false
	}

	sortCache := make([]int, len(s.ackCache))

	for k, v := range s.ackCache {
		sortCache[k] = int(v)
	}

	sort.Ints(sortCache)

	for k, v := range sortCache {
		s.ackCache[k] = uint32(v)
	}

loop:

	last := s.ackCache[0]
	var count uint32
	for i := 1; i < len(s.ackCache); i++ {

		if s.ackCache[i] == last {
			s.ackCache = s.ackCache[1:]
			goto loop
		}
		if s.ackCache[i] == last+1+count {
			count++
		} else {
			break
		}
	}

	if count != 0 {
		if count > 2 {

			if len(s.ackSendCache)+(3*4)+DEFAULT_HEAD_SIZE > DEFAULT_INIT_PACKSIZE {
				return true
			}

			if len(s.ackSendCache) > 0 && s.ackSendCache[len(s.ackSendCache)-1] == last {
				s.ackSendCache = append(s.ackSendCache[:len(s.ackSendCache)-1], []uint32{uint32(last + count)}...)
			} else {
				s.ackSendCache = append(s.ackSendCache, []uint32{uint32(last), uint32(last), uint32(last + count)}...)
			}
			s.ackCache = s.ackCache[count+1:]

		} else {
			for j := uint32(0); j <= count; j++ {
				if len(s.ackSendCache)+(1*4)+DEFAULT_HEAD_SIZE > DEFAULT_INIT_PACKSIZE {
					return true
				}

				if len(s.ackSendCache) == 0 || s.ackSendCache[len(s.ackSendCache)-1] != last+j {
					s.ackSendCache = append(s.ackSendCache, last+j)
				}
			}
			s.ackCache = s.ackCache[count+1:]
		}

	} else {
		if len(s.ackSendCache)+(1*4)+DEFAULT_HEAD_SIZE > DEFAULT_INIT_PACKSIZE {
			return true
		}

		if len(s.ackSendCache) == 0 || s.ackSendCache[len(s.ackSendCache)-1] != last {
			s.ackSendCache = append(s.ackSendCache, last)
		}
		s.ackCache = s.ackCache[1:]
	}

	if len(s.ackCache) != 0 {
		goto loop
	}

	return false
}

func unWrapACK(ids []uint32) []uint32 {

	result := make([]uint32, 0, len(ids))
	var last uint32
	for i := 0; i < len(ids); i++ {
		if last != 0 && ids[i] == last && i+1 < len(ids) {

			for j := last + 1; j <= ids[i+1]; j++ {
				result = append(result, j)
			}
			i++
			continue
		} else {
			result = append(result, ids[i])
		}
		last = ids[i]
	}

	return result
}

// receive ack
func (s *Sniper) handleAck(ids []uint32) {
	s.mu.Lock()

	ids = unWrapACK(ids)
	var exp bool
	for _, id := range ids {

		if id < atomic.LoadUint32(&s.sendWinId) {
			continue
		}

		if id == s.sendId-1 {
			exp = true
		}

		if id == s.rttSampId {
			s.rttSampId = 0
			rtt := time.Now().UnixNano() - s.rttTimeFlag

			// collect
			if rtt/s.rtt < 2 {
				s.calrto(rtt)
			}
		}

		index := int(id) - int(atomic.LoadUint32(&s.sendWinId))

		if index >= len(s.ammoBag) {
			continue
		}

		s.ammoBag[index] = nil
	}

	s.mu.Unlock()
	if exp {
		s.expandWin()
		s.shoot()
	}

	return
}

func (s *Sniper) zoomoutWin() {
	s.winSize = int32(float64(s.winSize) / 1.25)
	if s.winSize < s.minSize {
		s.winSize = s.minSize
	}
}

func (s *Sniper) expandWin() {
	s.winSize = int32(float64(s.minSize) * 1.25)
}

func (s *Sniper) Write(b []byte) (n int, err error) {
	if s.isClose {
		return 0, CLOSEERROR
	}

	now := time.Now()
	if !s.deadline.IsZero() && s.deadline.Before(now) {
		return 0, TIMEOUERROR
	}

	if !s.writeDeadline.IsZero() && s.writeDeadline.Before(now) {
		return 0, TIMEOUERROR
	}

	return s.writer(b)
}

func (s *Sniper) delaySend(b []byte) (n int, err error) {

	n = len(b)
	s.mu.Lock()
loop:

	select {
	case <-s.errorSign:
		s.mu.Unlock()
		return 0, s.errorContainer.Load().(error)

	case <-s.closeChan:
		s.mu.Unlock()
		return 0, CLOSEERROR

	default:
	}

	remain := int64(s.winSize)*(s.packageSize)*2 - int64(len(s.sendCache))

	if remain <= 0 {
		s.mu.Unlock()
		_ = s.writerBlocker.Block()
		s.mu.Lock()
		goto loop
	}

	if remain >= int64(n) {

		// if appending sendCache don't have enough cap , will malloc a new memory
		// old memory will be GC
		// if the slice cap too small , then malloc new memory often happen , this will affect performance
		s.sendCache = append(s.sendCache, b...)
		s.addEffectFlow(len(b))

		if len(s.ammoBag) == 0 {
			s.mu.Unlock()
			s.shoot()
		} else {
			s.mu.Unlock()
		}

		return
	}

	s.sendCache = append(s.sendCache, b[:remain]...)

	b = b[remain:]

	goto loop
}

func (s *Sniper) monitor() {
	b := make([]byte, DEFAULT_INIT_PACKSIZE+20)
	for {

		select {
		case <-s.closeChan:
			return

		default:
			n, err := s.conn.Read(b)
			if err != nil {
				s.errorContainer.Store(errors.New(err.Error()))
				s.chanCloser.closeChan(s.errorSign)
				continue
			}

			msg, err := protocol.Unmarshal(b[:n])
			if err != nil {
				// bad message
				continue
			}

			routing(s, msg)
		}
	}
}

func (s *Sniper) Close() error {

	s.clock.Lock()
	defer s.clock.Unlock()

	var try int

loop:
	if s.isClose {
		return errors.New("is closed")
	}

	s.mu.Lock()
	// if ammoBag not clear , should delay try again
	if (len(s.ammoBag) == 0 && len(s.sendCache) == 0) || try > 0xff {
		s.mu.Unlock()

		s.ammoBag = append(s.ammoBag, &protocol.Ammo{
			Id:   atomic.LoadUint32(&s.sendId),
			Kind: protocol.CLOSE,
			Body: nil,
		})

		s.writerBlocker.Close()

		timer := time.NewTimer(time.Second * DEFAULT_INIT_HANDSHACK_TIMEOUT)
		defer timer.Stop()

		select {
		case <-s.closeChan:
		case <-timer.C:

			if s.noLeader {
				_ = s.conn.Close()
			}
			s.chanCloser.closeChan(s.closeChan)
		}

		s.writerBlocker.Close()
		s.chanCloser.closeChan(s.readBlock)

	} else {
		s.mu.Unlock()
		s.shoot()
		try++
		goto loop
	}

	s.isClose = true
	return nil
}
