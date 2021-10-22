package sharpshooter

import (
	"encoding/binary"
	"errors"
	"fmt"
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

const (
	STATUS_SECONDHANDSHACK = iota
	STATUS_THIRDHANDSHACK
	STATUS_NORMAL
	STATUS_CLOSEING1
	STATUS_CLOSEING2
	STATUS_CLOSEING3
)

var (
	CLOSEERROR         = errors.New("the connection is closed")
	HEALTHTIMEOUTERROR = errors.New("health monitor timeout ")
	TIMEOUERROR        = timeout(0)
	RCVAMMOBAGEMPTY    = make([]*protocol.Ammo, DEFAULT_INIT_RECEWIND)
)

type timeout uint

func (t timeout) Error() string {
	return "i/o timeout"
}

func (t timeout) Timeout() bool {
	return true
}

type Sniper struct {
	Statistics
	packageSize int64
	rtt         int64
	rto         int64
	rttTimeFlag int64

	interval   int64
	timeAnchor int64

	winSize        int32
	minSize        int32
	healthTryCount int32
	shootStatus    int32
	status         int32
	sendId         uint32
	latestSendId   uint32
	rcvId          uint32
	sendWinId      uint32
	rttSampId      uint32

	isClose   bool
	noLeader  bool // it not has headquarters
	staSwitch bool // statistics switch
	debug     bool

	sendBuffer []byte
	writer     func(p []byte) (n int, err error)

	//dead line
	readDeadline  time.Time
	deadline      time.Time
	writeDeadline time.Time

	aim  *net.UDPAddr
	conn *net.UDPConn

	ammoBag       []*protocol.Ammo
	rcvAmmoBag    []*protocol.Ammo
	rcvBuffer     [][]byte
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

	ammoPool sync.Pool

	chanCloser chanCloser
}

type Statistics struct {
	TotalTraffic     int64
	EffectiveTraffic int64
	TotalPacket      int64
	EffectivePacket  int64
	RTT              int64
	RTO              int64
	SendWin          int64
	ReceiveWin       int64
}

func (s *Sniper) LocalAddr() net.Addr {
	return s.conn.LocalAddr()
}

func (s *Sniper) Debug() {
	s.debug = true
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
		sendBuffer:    make([]byte, 0),
		interval:      DEFAULT_INIT_INTERVAL,
		minSize:       64,
	}

	sn.rcv = sn.rcvnoml

	sn.wrap = sn.wrapnoml

	sn.writer = sn.delaySend

	sn.ammoPool.New = func() interface{} {
		ammo := &protocol.Ammo{
			Body: make([]byte, sn.packageSize),
		}
		ammo.Body = ammo.Body[:0]
		return ammo
	}
	return sn
}

func (s *Sniper) addTotalTraffic(flow int) {
	if s.staSwitch {
		s.TotalTraffic += int64(flow)
	}
}

func (s *Sniper) addTotalPacket(n int) {
	if s.staSwitch {
		s.TotalPacket += int64(n)
	}
}

func (s *Sniper) addEffectivePacket(n int) {
	if s.staSwitch {
		s.EffectivePacket += int64(n)
	}
}

func (s *Sniper) CleanStatistics() {
	s.EffectivePacket = 0
	s.EffectiveTraffic = 0
	s.TotalPacket = 0
	s.TotalTraffic = 0
}

func (s *Sniper) addEffectTraffic(flow int) {
	if s.staSwitch {
		s.EffectiveTraffic += int64(flow)
	}
}

func (s *Sniper) TrafficStatistics() Statistics {
	s.RTO = s.rto
	s.RTT = s.rtt
	s.SendWin = int64(s.winSize)
	s.ReceiveWin = int64(len(s.rcvAmmoBag))
	return s.Statistics
}

func (s *Sniper) SetPackageSize(size int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if size == 0 {
		size = 512
	}
	s.packageSize = size

	s.ammoPool.New = func() interface{} {
		ammo := &protocol.Ammo{
			Body: make([]byte, s.packageSize),
		}
		ammo.Body = ammo.Body[:0]
		return ammo
	}
}

func (s *Sniper) SetRecWin(size int64) {
	s.bemu.Lock()
	defer s.bemu.Unlock()
	s.rcvAmmoBag = make([]*protocol.Ammo, size)
	if size > int64(len(RCVAMMOBAGEMPTY)) {
		s.rcvAmmoBag = make([]*protocol.Ammo, size)
	}
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

func (s *Sniper) OpenStaTraffic() {
	s.staSwitch = true
}

// OpenFec use FEC algorithm in communication
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

func (s *Sniper) autoShoot() {
	s.shoot(true)
}

// send to remote
func (s *Sniper) shoot(put bool) {

	now := time.Now()
	if put {
		rto := atomic.LoadInt64(&s.rto)
		// maybe overflow
		if rto <= 0 {
			rto = int64(250 * time.Millisecond)
		}

		interval := time.Duration(
			math.Min(float64(rto),
				float64(time.Duration(s.interval)*time.Millisecond)),
		) * time.Nanosecond
		if s.debug {
			fmt.Printf("interval :%d  \n", interval)
		}
		shootTime := now.Add(interval)

		SystemTimedSched.Put(s.autoShoot, shootTime)

		// avoid sending data repeatedly
		if now.UnixNano()-s.timeAnchor < int64(interval) && s.timeAnchor != 0 {
			return
		}
	}

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

		if put && s.timeAnchor != 0 {

			//packet loss tolerance
			var c int
			for _, ammo := range s.ammoBag {
				if ammo != nil && int(ammo.Id)-int(s.latestSendId) > 0 {
					break
				}
				if ammo != nil {
					c++
				}
			}

			if float64(c)/float64(s.winSize) > 0.80 {
				s.zoomoutWin()
			}
		}

		s.wrap()

		if len(s.ammoBag) == 0 {
			s.mu.Unlock()
			s.timeAnchor = 0
			return
		}

		for k := range s.ammoBag {

			if s.ammoBag[k] == nil {
				continue
			}

			if k > int(s.winSize) {
				break
			}
			id := s.ammoBag[k].Id
			if id%uint32(s.winSize) == 0 && s.rttSampId < id {
				// samp
				s.rttSampId = s.ammoBag[k].Id
				s.rttTimeFlag = now.UnixNano()
			}

			b := protocol.Marshal(*s.ammoBag[k])

			if s.debug {
				fmt.Printf("%d	send to %s , seq:%d \n", now.UnixNano(), s.aim.String(), s.ammoBag[k].Id)
			}
			_, err := s.conn.WriteToUDP(b, s.aim)
			if err != nil {
				select {
				case <-s.errorSign:
				default:
					s.errorContainer.Store(errors.New(err.Error()))
					s.chanCloser.closeChan(s.errorSign)
				}
			}

			s.latestSendId = s.ammoBag[k].Id
			s.addTotalTraffic(len(b))
			s.addTotalPacket(1)
		}

		s.timeAnchor = now.UnixNano()
		s.mu.Unlock()
	}
}

// remove already sent packages
func (s *Sniper) flush() {

	defer func() {
		remain := (s.packageSize) - int64(len(s.sendBuffer))
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
		// repeat id
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

	interval := time.Duration(
		math.Min(float64(s.rtt/4),
			float64(30*time.Millisecond)),
	) * time.Nanosecond

	SystemTimedSched.Put(s.ackTimer, time.Now().Add(interval*time.Nanosecond))
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

		if s.debug {
			var seqs []uint32
			for i := 0; i < len(ammo.Body); i += 4 {
				seqs = append(seqs, binary.BigEndian.Uint32(ammo.Body[i:i+4]))
			}
			fmt.Printf("%d	ack send to %s, ack list :%v\n", time.Now().UnixNano(), s.aim.String(), seqs)
		}

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

	now := time.Now()
	if s.debug {
		fmt.Printf("%d	ack receive to %s ,ack seqs:%v \n", now.UnixNano(), s.aim.String(), ids)
	}
	s.mu.Lock()

	ids = unWrapACK(ids)
	var exp bool

	for _, id := range ids {

		if id < atomic.LoadUint32(&s.sendWinId) {
			continue
		}

		if id == s.rttSampId {
			s.rttSampId = 0
			rtt := now.UnixNano() - s.rttTimeFlag

			if s.debug {
				fmt.Printf("rtt :%d  rttflag %d\n", rtt, s.rttTimeFlag)
			}
			// collect
			if rtt/s.rtt < 5 {
				s.calrto(rtt)
			}
		}

		index := int(id) - int(atomic.LoadUint32(&s.sendWinId))

		if index >= len(s.ammoBag) {
			continue
		}

		if s.ammoBag[index] != nil {
			s.ammoBag[index].Body = s.ammoBag[index].Body[:0]
			s.ammoPool.Put(s.ammoBag[index])
			s.ammoBag[index] = nil
		}

		if id == s.latestSendId {
			exp = true
			for i := range s.ammoBag {
				if s.ammoBag[i] != nil {
					exp = false
					break
				}
			}
		}
	}

	for i := 0; i < int(ids[len(ids)-1])-int(atomic.LoadUint32(&s.sendWinId)); i++ {
		if s.ammoBag[i] != nil {
			b := protocol.Marshal(*s.ammoBag[i])

			if s.debug {
				fmt.Printf("%d	send to %s , seq:%d \n", now.UnixNano(), s.aim.String(), s.ammoBag[i].Id)
			}
			_, err := s.conn.WriteToUDP(b, s.aim)
			if err != nil {
				select {
				case <-s.errorSign:
				default:
					s.errorContainer.Store(errors.New(err.Error()))
					s.chanCloser.closeChan(s.errorSign)
				}
			}

			s.addTotalTraffic(len(b))
			s.addTotalPacket(1)
		}
	}

	s.mu.Unlock()
	if exp {
		s.expandWin()
		if s.debug {
			fmt.Printf("all message verified, shoot now! \n")
		}
		s.shoot(false)
	}

	return
}

func (s *Sniper) zoomoutWin() {
	s.winSize = int32(float64(s.winSize) / 1.25)
	if s.winSize < s.minSize {
		s.winSize = s.minSize
		if s.debug {
			fmt.Printf("winSize zoomout %d \n", s.winSize)
		}
	}
}

func (s *Sniper) expandWin() {
	if s.winSize < 1<<15 {
		s.winSize = int32(float64(s.winSize) * 1.25)
		if s.debug {
			fmt.Printf("winSize expand %d \n", s.winSize)
		}
	}
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
	s.addEffectTraffic(n)
	s.mu.Lock()
loop:

	ammoLen := len(s.ammoBag)

	select {
	case <-s.errorSign:
		s.mu.Unlock()
		return 0, s.errorContainer.Load().(error)

	case <-s.closeChan:
		s.mu.Unlock()
		return 0, CLOSEERROR

	default:
	}

	remain := s.winSize - int32(len(s.ammoBag))

	if remain <= 0 {
		s.mu.Unlock()
		_ = s.writerBlocker.Block()
		s.mu.Lock()
		goto loop
	}

	if len(s.sendBuffer) != 0 {
		r := s.packageSize - int64(len(s.sendBuffer))
		if r > int64(len(b)) {
			s.sendBuffer = append(s.sendBuffer, b...)
			b = b[:0]
		} else {
			s.sendBuffer = append(s.sendBuffer, b[:r]...)
			b = b[r:]
			s.wrap()
		}
	}

	for i := 0; i < int(remain); i++ {

		if len(b) == 0 {
			s.mu.Unlock()
			if ammoLen == 0 {
				if s.debug {
					fmt.Printf("ammobag lenght is 0 , shoot now! \n")
				}
				s.shoot(false)
			}

			return n, nil
		}

		if int64(len(b)) >= s.packageSize {

			id := atomic.AddUint32(&s.sendId, 1)
			ammo := s.ammoPool.Get().(*protocol.Ammo)
			ammo.Id = id - 1
			ammo.Kind = protocol.NORMAL
			ammo.Body = append(ammo.Body, b[:s.packageSize]...)
			b = b[s.packageSize:]

			s.addEffectivePacket(1)
			s.ammoBag = append(s.ammoBag, ammo)
		} else {
			s.sendBuffer = append(s.sendBuffer, b...)
			s.mu.Unlock()
			return n, nil
		}
	}

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
	if (len(s.ammoBag) == 0 && len(s.sendBuffer) == 0) || try > 0xff {
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
		s.shoot(false)
		try++
		goto loop
	}

	s.isClose = true
	return nil
}

func (s *Sniper) copyRcvBuffer(b []byte) int {

	s.bemu.Lock()
	defer s.bemu.Unlock()

	if len(s.rcvBuffer) == 0 {
		return 0
	}

	var total int
	var catIndedx int

	nb := b

	blen := len(b)
	for k := range s.rcvBuffer {
		n := copy(nb, s.rcvBuffer[k])
		total += n

		if n < len(s.rcvBuffer[k]) {
			s.rcvBuffer[k] = s.rcvBuffer[k][n:]
		} else if n == len(s.rcvBuffer[k]) {
			catIndedx++
		}
		if total == blen {
			break
		}
		nb = b[total:]
	}

	s.rcvBuffer = s.rcvBuffer[catIndedx:]
	return total
}

func removeByte(q []byte, n int) []byte {
	if n > cap(q)/2 {
		newn := copy(q, q[n:])
		return q[:newn]
	}
	return q[n:]
}
