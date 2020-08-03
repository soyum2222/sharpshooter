package sharpshooter

import (
	"encoding/binary"
	"errors"
	"github.com/soyum2222/sharpshooter/protocol"
	"github.com/soyum2222/sharpshooter/tool/block"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	shooting = 1 << iota
	waittimeout
)

const (
	DEFAULT_INIT_SENDWIND                      = 1024
	DEFAULT_INIT_RECEWIND                      = 1 << 10
	DEFAULT_INIT_PACKSIZE                      = 1024
	DEFAULT_INIT_HEALTHTICKER                  = 3
	DEFAULT_INIT_HEALTHCHECK_TIMEOUT_TRY_COUNT = 10
	DEFAULT_INIT_HANDSHACK_TIMEOUT             = 6
	DEFAULT_INIT_RTO_UNIT                      = float64(200 * time.Millisecond)
	DEFAULT_INIT_DELAY_ACK                     = float64(200 * time.Millisecond)
)

var (
	CLOSEERROR   = errors.New("the connection is closed")
	TIMEOUTERROR = errors.New("health monitor timeout ")
)

type Sniper struct {
	isDelay        bool
	isClose        bool
	noLeader       bool // it not has headquarters
	staSwitch      bool // statistics switch
	maxWin         int32
	healthTryCount int32
	shootStatus    int32
	sendId         uint32
	rcvId          uint32
	sendWinId      uint32
	sendBottom     uint32
	//placeholder    int32 // in 32 bit OS must 8-byte alignment, this field itself has no meaning
	packageSize   int64
	rtt           int64
	rto           int64
	timeFlag      int64
	totalFlow     int64 // statistics total flow used
	effectiveFlow int64 // statistics effective flow
	sendCache     []byte
	writer        func(p []byte) (n int, err error)

	aim            *net.UDPAddr
	conn           *net.UDPConn
	timeoutTimer   *time.Timer
	healthTimer    *time.Timer
	ammoBag        []*protocol.Ammo
	rcvAmmoBag     []*protocol.Ammo
	rcvCache       []byte
	readBlock      chan struct{}
	handShakeSign  chan struct{}
	ackSign        *block.Blocker
	writerBlocker  *block.Blocker
	closeChan      chan struct{}
	stopShotSign   chan struct{}
	errorSign      chan struct{}
	fece           *fecEncoder
	fecd           *fecDecoder
	errorContainer atomic.Value
	ackCache       []uint32

	// ack lock
	ackLock sync.Mutex
	// send lock
	mu sync.Mutex
	// receive lock
	bemu sync.Mutex
	// close lock
	clock sync.Mutex
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
		maxWin:        DEFAULT_INIT_SENDWIND,
		mu:            sync.Mutex{},
		bemu:          sync.Mutex{},
		packageSize:   DEFAULT_INIT_PACKSIZE,
		ackSign:       block.NewBlocker(),
		writerBlocker: block.NewBlocker(),
		closeChan:     make(chan struct{}, 0),
		stopShotSign:  make(chan struct{}, 0),
		errorSign:     make(chan struct{}),
		sendCache:     make([]byte, 0),
		fecd:          newFecDecoder(4, 1),
		fece:          newFecEncoder(4, 1),
	}

	sn.isDelay = true
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

func (s *Sniper) SetPackageSize(size int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.packageSize = size
}

func (s *Sniper) SetRecWin(size int64) {
	s.bemu.Lock()
	defer s.bemu.Unlock()
	s.rcvAmmoBag = make([]*protocol.Ammo, size)
}

func (s *Sniper) SetSendWin(size int32) {
	s.bemu.Lock()
	defer s.bemu.Unlock()
	s.maxWin = size
}

func (s *Sniper) OpenStaFlow() {
	s.staSwitch = true
}

func (s *Sniper) healthMonitor() {

	for {

		select {
		case <-s.healthTimer.C:

			if s.healthTryCount < DEFAULT_INIT_HEALTHCHECK_TIMEOUT_TRY_COUNT {

				if s.healthTryCount == 0 {
					s.timeFlag = time.Now().UnixNano()
				}

				_, _ = s.conn.WriteToUDP(protocol.Marshal(protocol.Ammo{
					Id:   0,
					Kind: protocol.HEALTHCHECK,
				}), s.aim)

				atomic.AddInt32(&s.healthTryCount, 1)

				s.healthTimer.Reset(time.Second * DEFAULT_INIT_HEALTHTICKER)

			} else {
				// timeout
				s.clock.Lock()
				defer s.clock.Unlock()
				if s.isClose {
					return
				}

				s.errorContainer.Store(errors.New(TIMEOUTERROR.Error()))
				close(s.errorSign)

				s.ackSign.Close()
				s.writerBlocker.Close()

				if s.noLeader {
					_ = s.conn.Close()
				}

				s.isClose = true
				close(s.closeChan)
				close(s.readBlock)

				return
			}

		case _, _ = <-s.closeChan:
			// if come here , then close func has been executed
			return
		}
	}

}

// send to remote
func (s *Sniper) shoot() {
	if atomic.CompareAndSwapInt32(&s.shootStatus, 0, shooting) {

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

		var currentWindowEndId uint32

		for k := range s.ammoBag {

			if s.ammoBag[k] == nil {
				continue
			}

			if k > int(s.maxWin) {
				break
			}

			currentWindowEndId = s.ammoBag[k].Id

			b := protocol.Marshal(*s.ammoBag[k])

			_, err := s.conn.WriteToUDP(b, s.aim)
			if err != nil {
				select {
				case <-s.errorSign:
				default:
					s.errorContainer.Store(errors.New(err.Error()))
					close(s.errorSign)
				}
			}

			s.addTotalFlow(len(b))

		}

		atomic.StoreUint32(&s.sendBottom, currentWindowEndId)

		s.mu.Unlock()

		atomic.StoreInt32(&s.shootStatus, 0)

	}
	rto := atomic.LoadInt64(&s.rto)

	// Maybe overflow
	if rto <= 0 {
		rto = int64(250 * time.Millisecond)
	}

	s.timeoutTimer.Reset(time.Duration(math.Min(float64(rto), float64(1000*time.Millisecond))) * time.Nanosecond)
}

// remove already sent packages
func (s *Sniper) flush() {

	defer func() {
		remain := int64(s.maxWin)*(s.packageSize)*2 - int64(len(s.sendCache))
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

// timed trigger send
func (s *Sniper) shooter() {

	for {

		select {

		case <-s.timeoutTimer.C:

			s.rto = s.rto * 2
			if s.maxWin > 1<<4 {
				s.zoomoutWin()
			}

			break

		case <-s.closeChan:
			return
		}

		s.shoot()

	}

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
		s._ackSender()
	}

	_ = s.ackSign.Pass()

}

// timed trigger send ack
func (s *Sniper) ackTimer() {

	timer := time.NewTimer(time.Duration(DEFAULT_INIT_DELAY_ACK))
	for {

		s._ackSender()

		select {
		case <-timer.C:
		case <-s.closeChan:
			return
		}

		timer.Reset(time.Duration(s.rtt / 2))
	}
}

func (s *Sniper) _ackSender() {
	s.ackLock.Lock()
	defer s.ackLock.Unlock()

	l := len(s.ackCache)
	if l != 0 {

		b := make([]byte, l*4)

		for i := 0; i < len(s.ackCache); i++ {
			binary.BigEndian.PutUint32(b[4*i:(i+1)*4], s.ackCache[i])
		}

		ammo := protocol.Ammo{
			Kind: protocol.ACK,
			Body: b,
		}

		s.ackCache = nil

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

// receive ack
func (s *Sniper) handleAck(ids []uint32) {
	s.mu.Lock()

	for _, id := range ids {

		if id < atomic.LoadUint32(&s.sendWinId) {
			s.mu.Unlock()
			return
		}

		index := int(id) - int(atomic.LoadUint32(&s.sendWinId))

		if index >= len(s.ammoBag) {
			s.mu.Unlock()
			return
		}
		s.ammoBag[index] = nil
	}

	s.mu.Unlock()
	s.shoot()

	return
}

func (s *Sniper) zoomoutWin() {
	s.maxWin -= 1
}

func (s *Sniper) expandWin() {
	s.maxWin += 1
}

func (s *Sniper) beShot(ammo *protocol.Ammo) {

	s.bemu.Lock()
	defer s.bemu.Unlock()

	// eg: int8(0000 0000) - int8(1111 1111) = int8(0 - -1) = 1
	// eg: int8(1000 0001) - int8(1000 0000) = int8(-127 - -128) = 1
	// eg: int8(1000 0000) - int8(0111 1111) = int8(-128 - 127) = 1
	bagIndex := int(ammo.Id) - int(s.rcvId)

	// id < currentId , lost the ack

	s.ack(ammo.Id)

	if bagIndex >= len(s.rcvAmmoBag) {
		return
	}

	if s.rcvAmmoBag[bagIndex] == nil {
		s.rcvAmmoBag[bagIndex] = ammo
	}

	var anchor int
	for i := 0; i < len(s.rcvAmmoBag); {

		blocks := make([][]byte, s.fecd.dataShards+s.fecd.parShards)
		var empty int

		for j := 0; j < s.fecd.dataShards+s.fecd.parShards && i < len(s.rcvAmmoBag); j++ {

			if s.rcvAmmoBag[i] == nil {
				blocks[j] = nil
				empty++
			} else {
				blocks[j] = s.rcvAmmoBag[i].Body
			}

			i++
		}

		if empty > s.fecd.parShards {
			break
		}

		data, err := s.fecd.decode(blocks)
		if err == nil {

			anchor += s.fecd.dataShards + s.fecd.parShards

			s.rcvCache = append(s.rcvCache, data...)

			s.rcvId += uint32(s.fecd.dataShards + s.fecd.parShards)

		} else {
			break
		}
	}

	// cut off
	if anchor > len(s.rcvAmmoBag) {
		s.rcvAmmoBag = s.rcvAmmoBag[:0]
	} else {
		s.rcvAmmoBag = s.rcvAmmoBag[anchor:]
		s.rcvAmmoBag = append(s.rcvAmmoBag, make([]*protocol.Ammo, anchor)...)
	}

	if s.isClose {
		return
	}

	select {

	case s.readBlock <- struct{}{}:

	default:

	}
}

func (s *Sniper) Read(b []byte) (n int, err error) {

loop:

	s.bemu.Lock()

	n += copy(b, s.rcvCache)
	s.rcvCache = s.rcvCache[n:]

	s.bemu.Unlock()

	if n <= 0 {
		select {

		case _, ok := <-s.readBlock:
			if !ok {
				return 0, CLOSEERROR
			}
			goto loop

		case _, _ = <-s.closeChan:
			return 0, CLOSEERROR

		case <-s.errorSign:
			return 0, s.errorContainer.Load().(error)

		}

	}

	return
}

func (s *Sniper) wrap() {

loop:
	remain := s.maxWin - int32(len(s.ammoBag))

	if remain <= 0 {
		return
	}

	l := len(s.sendCache)

	if l == 0 {
		return
	}

	// anchor is mark sendCache current op index
	var anchor int64

	if int64(l) < int64(s.fece.dataShards)*s.packageSize {
		anchor = int64(l)
	} else {
		anchor = int64(s.fece.dataShards) * s.packageSize
	}

	body := s.sendCache[:anchor]

	// body will be copied
	shard, err := s.fece.encode(body)
	if err != nil {
		s.errorContainer.Store(err)
		select {
		case <-s.errorSign:
		default:
			s.errorContainer.Store(errors.New(err.Error()))
			close(s.errorSign)
		}
		return
	}

	s.sendCache = s.sendCache[anchor:]

	for _, v := range shard {

		id := atomic.AddUint32(&s.sendId, 1)

		ammo := protocol.Ammo{
			Id:   id - 1,
			Kind: protocol.NORMAL,
			Body: v,
		}

		s.ammoBag = append(s.ammoBag, &ammo)

	}

	goto loop

}

func (s *Sniper) Write(b []byte) (n int, err error) {
	if s.isClose {
		return 0, CLOSEERROR
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
	remain := int64(s.maxWin)*(s.packageSize)*2 - int64(len(s.sendCache))

	if remain <= 0 {
		s.mu.Unlock()
		_ = s.writerBlocker.Block()
		s.mu.Lock()
		goto loop
	}

	if remain >= int64(len(b)) {

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
				select {
				case <-s.errorSign:
				default:
					close(s.errorSign)
				}
				continue
			}

			msg := protocol.Unmarshal(b[:n])
			routing(s, msg)
		}

	}
}

func (s *Sniper) Close() {

	s.clock.Lock()
	defer s.clock.Unlock()

	var try int

loop:
	if s.isClose || try > 10 {
		return
	}

	s.mu.Lock()
	// if ammoBag not clear , should delay try again
	if len(s.ammoBag) == 0 && len(s.sendCache) == 0 {
		s.mu.Unlock()

		s.ammoBag = append(s.ammoBag, &protocol.Ammo{
			Id:   atomic.LoadUint32(&s.sendId),
			Kind: protocol.CLOSE,
			Body: nil,
		})

		s.writerBlocker.Close()

		select {
		case <-s.closeChan:
		case <-time.NewTimer(time.Second * DEFAULT_INIT_HANDSHACK_TIMEOUT).C:
			s.isClose = true
			if s.noLeader {
				_ = s.conn.Close()
			}
			close(s.closeChan)
		}

		s.writerBlocker.Close()
		s.ackSign.Close()
		select {
		case <-s.readBlock:
		default:
			close(s.readBlock)
		}

		s.isClose = true

	} else {
		s.mu.Unlock()
		time.Sleep(time.Second)
		try++
		goto loop

	}

	s.isClose = true

}
