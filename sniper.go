package sharpshooter

import (
	"errors"
	"fmt"

	//"fmt"
	"net"
	"sharpshooter/protocol"
	"sharpshooter/tool/block"
	"sync"
	"sync/atomic"
	"time"
)

const (
	shooting = 1 << iota
	waittimeout
)

var Flow int

//var mark map[uint32]bool

const (
	DEFAULT_INIT_SENDWIND                      = 1024
	DEFAULT_INIT_MAXSENDWIND                   = 1024 * 10
	DEFAULT_INIT_RECEWIND                      = 1 << 10
	DEFAULT_INIT_PACKSIZE                      = 1024
	DEFAULT_INIT_HEALTHTICKER                  = 3
	DEFAULT_INIT_HEALTHCHECK_TIMEOUT_TRY_COUNT = 10
	//DEFAULT_INIT_SENDCACH                      = 1 << 10
	DEFAULT_INIT_RTO_UNIT  = float64(500 * time.Millisecond)
	DEFAULT_INIT_DELAY_ACK = float64(200 * time.Millisecond)
)

var (
	CLOSEERROR   = errors.New("the connection is closed")
	TIMEOUTERROR = errors.New("health monitor timeout ")
)

type Sniper struct {
	isdefer              bool
	isClose              bool
	maxWindows           int32
	oldWindows           int32
	healthTryCount       int32
	sendid               uint32
	beShotCurrentId      uint32
	currentWindowStartId uint32
	currentWindowEndId   uint32
	shootStatus          int32
	ackId                uint32
	index                uint32
	latestAckId          uint32
	packageSize          int
	lostCount            int
	//placeholder          int32 // in 32 bit OS must 8-byte alignment
	rtt      int64
	rto      int64
	timeFlag int64
	//sendCacheSize  int64
	sendCache      []byte
	writer         func(p []byte) (n int, err error)
	aim            *net.UDPAddr
	conn           *net.UDPConn
	timeoutTimer   *time.Timer
	healthTimer    *time.Timer
	ammoBag        []*protocol.Ammo
	beShotAmmoBag  []*protocol.Ammo
	ammoBagCach    chan protocol.Ammo
	readblock      chan struct{}
	handshakesign  chan struct{}
	acksign        *block.Blocker
	writerBlocker  *block.Blocker
	closeChan      chan struct{}
	stopshotsign   chan struct{}
	errorsign      chan struct{}
	errorcontainer atomic.Value
	mu             sync.RWMutex
	bemu           sync.Mutex
	clock          sync.Mutex
}

func NewSniper(conn *net.UDPConn, aim *net.UDPAddr) *Sniper {

	sn := &Sniper{
		aim:           aim,
		conn:          conn,
		rtt:           int64(time.Millisecond * 500),
		rto:           int64(time.Second),
		beShotAmmoBag: make([]*protocol.Ammo, DEFAULT_INIT_RECEWIND),
		handshakesign: make(chan struct{}, 1),
		readblock:     make(chan struct{}, 0),
		maxWindows:    DEFAULT_INIT_SENDWIND,
		mu:            sync.RWMutex{},
		bemu:          sync.Mutex{},
		packageSize:   DEFAULT_INIT_PACKSIZE,
		acksign:       block.NewBlocker(),
		writerBlocker: block.NewBlocker(),
		closeChan:     make(chan struct{}, 0),
		stopshotsign:  make(chan struct{}, 0),
		errorsign:     make(chan struct{}),
		sendCache:     make([]byte, 0),
		//sendCacheSize: DEFAULT_INIT_SENDCACH,
	}
	sn.writer = sn.deferSend
	return sn
}

//
//func (s *Sniper) SetSendCache(size int64) {
//	s.mu.Lock()
//	defer s.mu.Unlock()
//	s.sendCacheSize = size
//}
//
//func (s *Sniper) SetPackageSize(size int64) {
//	s.mu.Lock()
//	defer s.mu.Unlock()
//	s.packageSize = size
//}
//
//func (s *Sniper) SetRecWin(size int64) {
//	s.bemu.Lock()
//	defer s.bemu.Unlock()
//	s.beShotAmmoBag = make([]*protocol.Ammo, size)
//}

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

				s.errorcontainer.Store(errors.New(TIMEOUTERROR.Error()))
				close(s.errorsign)

				s.acksign.Close()
				s.writerBlocker.Close()
				//s.conn.Close()
				s.isClose = true
				close(s.closeChan)
				close(s.readblock)

				return
			}

		case _, _ = <-s.closeChan:
			// if come here , then close has been executed
			return
		}
	}

}

func (s *Sniper) shoot() {
	if atomic.CompareAndSwapInt32(&s.shootStatus, 0, shooting) {

		select {
		case <-s.errorsign:
			return
		case <-s.closeChan:
			return
		default:

		}

		s.mu.Lock()

		s.flush()

		s.wrap()

		var currentWindowEndId uint32

		// send an ack
		id := atomic.LoadUint32(&s.ackId)

		if id != 0 {
			ammo := protocol.Ammo{
				Id:   id,
				Kind: protocol.ACK,
				Body: nil,
			}

			_, _ = s.conn.WriteToUDP(protocol.Marshal(ammo), s.aim)
		}

		for k, _ := range s.ammoBag {

			if s.ammoBag[k] == nil {
				continue
			}

			if k > int(s.maxWindows) {
				break
			}

			currentWindowEndId = s.ammoBag[k].Id

			b := protocol.Marshal(*s.ammoBag[k])

			_, err := s.conn.WriteToUDP(b, s.aim)
			if err != nil {
				select {
				case <-s.errorsign:
				default:
					s.errorcontainer.Store(errors.New(err.Error()))
					close(s.errorsign)
				}
			}

			Flow += len(b)

		}

		atomic.StoreUint32(&s.currentWindowEndId, currentWindowEndId)

		s.mu.Unlock()

		atomic.StoreInt32(&s.shootStatus, 0)
	}
	rto := atomic.LoadInt64(&s.rto)

	s.timeoutTimer.Reset(time.Duration(rto) * time.Nanosecond)
}

func (s *Sniper) flush() {
	if len(s.ammoBag) < int(s.index) || s.index == 0 {
		return
	}
	s.ammoBag = s.ammoBag[s.index:]

	if len(s.ammoBag) == 0 {
		atomic.AddUint32(&s.currentWindowStartId, s.index)
		atomic.StoreUint32(&s.index, 0)
		return
	}

	atomic.StoreUint32(&s.currentWindowStartId, s.ammoBag[0].Id)
	atomic.StoreUint32(&s.index, 0)

}

func (s *Sniper) shooter() {

	s.timeoutTimer = time.NewTimer(time.Duration(s.rto) * time.Nanosecond)
	for {

		select {

		case <-s.timeoutTimer.C:

			s.rto = s.rto * 2
			if s.maxWindows > 1<<4 {
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
	cid := atomic.LoadUint32(&s.ackId)
	if cid < id {
		atomic.CompareAndSwapUint32(&s.ackId, cid, id)
	}

	s.acksign.Pass()

}

func (s *Sniper) ackSender() {

	timer := time.NewTimer(time.Duration(DEFAULT_INIT_DELAY_ACK))
	for {

		id := atomic.LoadUint32(&s.ackId)
		if id == 0 {

			err := s.acksign.Block()
			if err != nil {
				return
			}
			continue

		}

		ammo := protocol.Ammo{
			Id:   id,
			Kind: protocol.ACK,
			Body: nil,
		}

		_, err := s.conn.WriteToUDP(protocol.Marshal(ammo), s.aim)
		if err != nil {
			select {
			case <-s.errorsign:
				return
			default:
				s.errorcontainer.Store(errors.New(err.Error()))
			}
		}

		//atomic.CompareAndSwapUint32(&s.ackId, id, 0)

		select {
		case <-timer.C:
		case <-s.closeChan:
			return
		}

		timer.Reset(time.Duration(s.rtt))

	}
}

func (s *Sniper) score(id uint32) {
	//fmt.Println("score ", id)
	s.mu.Lock()

	if id < atomic.LoadUint32(&s.currentWindowStartId) {
		s.mu.Unlock()
		return
	}

	index := int(id) - int(atomic.LoadUint32(&s.currentWindowStartId))

	if index > len(s.ammoBag) {
		s.mu.Unlock()
		return
	}

	atomic.StoreUint32(&s.index, uint32(index))

	if id == s.latestAckId {
		s.lostCount++

		if s.lostCount > 2 {

			//if index >= len(s.ammoBag) {
			//	_ = s.writerBlocker.Pass()
			//	_, _ = s.conn.WriteTo(protocol.Marshal(protocol.Ammo{
			//		Kind: protocol.OUTOFAMMO,
			//		Body: nil,
			//	}), s.aim)
			//	s.mu.Unlock()
			//	return
			//}

			s.zoomoutWin()

			s.lostCount = 0
			s.mu.Unlock()
			s.shoot()
			return
		}

	} else {
		s.lostCount = 0
		s.latestAckId = id
	}

	// now send window is send clean
	if id >= atomic.LoadUint32(&s.currentWindowEndId) {

		s.expandWin()

		_ = s.writerBlocker.Pass()

		if len(s.ammoBag) > 0 {
			s.mu.Unlock()
			s.shoot()
		} else {
			s.mu.Unlock()
		}

	} else {
		s.mu.Unlock()
	}

	return

}

func (s *Sniper) zoomoutWin() {
	//old := s.maxWindows
	//
	//s.maxWindows -= int32(math.Abs(float64(s.maxWindows-s.oldWindows)) / 2)
	//s.oldWindows = old
	s.maxWindows -= 1
	fmt.Println("win", s.maxWindows)

}

func (s *Sniper) expandWin() {

	s.maxWindows += 1
	//s.maxWindows += int32(math.Abs(float64(s.maxWindows-s.oldWindows)) / 2)
	fmt.Println("win", s.maxWindows)
}

func (s *Sniper) beShot(ammo *protocol.Ammo) {

	//if ammo.Kind == protocol.OUTOFAMMO {
	//	atomic.StoreUint32(&s.ackId, 0)
	//	return
	//}

	s.bemu.Lock()
	defer s.bemu.Unlock()

	bagIndex := ammo.Id - s.beShotCurrentId

	// id < currentId , lost the ack
	if ammo.Id < s.beShotCurrentId {

		var id uint32
		for k, v := range s.beShotAmmoBag {

			if v == nil {
				id = s.beShotCurrentId + uint32(k)
				break
			}
		}

		if id == 0 {
			id = s.beShotCurrentId + uint32(len(s.beShotAmmoBag))
		}

		s.ack(id)

		return
	}

	if int(bagIndex) >= len(s.beShotAmmoBag) {
		return
	}

	if s.beShotAmmoBag[bagIndex] == nil {
		s.beShotAmmoBag[bagIndex] = ammo
	}

	var nid uint32
	for k, v := range s.beShotAmmoBag {

		if v == nil {
			nid = s.beShotCurrentId + uint32(k)
			break
		}

	}

	if nid == 0 {
		nid = s.beShotCurrentId + uint32(len(s.beShotAmmoBag))
	}

	s.ack(nid)

	if s.isClose {
		return
	}

	select {

	case s.readblock <- struct{}{}:

	default:

	}
}

func (s *Sniper) Read(b []byte) (n int, err error) {
loop:

	s.bemu.Lock()
	for len(s.beShotAmmoBag) != 0 && s.beShotAmmoBag[0] != nil {

		cpn := copy(b[n:], s.beShotAmmoBag[0].Body)

		n += cpn

		if n == len(b) {
			if cpn < len(s.beShotAmmoBag[0].Body) {
				s.beShotAmmoBag[0].Body = s.beShotAmmoBag[0].Body[cpn:]
			} else {
				s.beShotAmmoBag = append(s.beShotAmmoBag[1:], nil)
				s.beShotCurrentId++
			}
			break

		} else {

			s.beShotAmmoBag = append(s.beShotAmmoBag[1:], nil)
			s.beShotCurrentId++
			continue
		}

	}

	s.bemu.Unlock()

	if n <= 0 {
		select {

		case <-s.readblock:
			goto loop

		case _, _ = <-s.closeChan:
			return 0, CLOSEERROR

		case <-s.errorsign:
			return 0, s.errorcontainer.Load().(error)

		}

	}

	return
}

func (s *Sniper) wrap() {
	remain := s.maxWindows - int32(len(s.ammoBag))

	for i := 0; i < int(remain); i++ {

		l := len(s.sendCache)

		if l == 0 {
			return
		}

		// anchor is mark sendCache current op index
		var anchor int

		if l < s.packageSize {
			anchor = l
		} else {
			anchor = s.packageSize
		}

		body := s.sendCache[:anchor]

		b := make([]byte, len(body))
		//b := make([]byte, 0, len(body))

		copy(b, body)
		//b = append(b, body...)

		s.sendCache = s.sendCache[anchor:]

		id := atomic.AddUint32(&s.sendid, 1)

		ammo := protocol.Ammo{
			Id:   id - 1,
			Kind: protocol.NORMAL,
			Body: b,
		}

		s.ammoBag = append(s.ammoBag, &ammo)

	}

}

func (s *Sniper) write(b []byte) (n int, err error) {

	id := atomic.AddUint32(&s.sendid, 1)

	select {

	case s.ammoBagCach <- protocol.Ammo{
		Length: 0,
		Id:     id - 1,
		Kind:   protocol.NORMAL,
		Body:   b,
	}:
	case <-s.errorsign:
		return 0, s.errorcontainer.Load().(error)

	}

	return len(b), nil
}

func (s *Sniper) Write(b []byte) (n int, err error) {
	if s.isClose {
		return 0, CLOSEERROR
	}
	return s.writer(b)
}

func (s *Sniper) OpenDeferSend() {
	s.writer = s.deferSend
	s.isdefer = true
}

func (s *Sniper) deferSend(b []byte) (n int, err error) {

	n = len(b)
	s.mu.Lock()
loop:

	select {
	case <-s.errorsign:
		return 0, s.errorcontainer.Load().(error)

	case <-s.closeChan:
		return 0, CLOSEERROR

	default:

	}
	remain := int64(s.maxWindows)*int64(s.packageSize)*2 - int64(len(s.sendCache))

	if remain <= 0 {
		s.mu.Unlock()
		s.writerBlocker.Block()
		s.mu.Lock()
		goto loop
	}

	if remain >= int64(len(b)) {
		s.sendCache = append(s.sendCache, b...)
		s.mu.Unlock()
		return
	}

	s.sendCache = append(s.sendCache, b[:remain]...)

	b = b[remain:]

	goto loop

}

func (s *Sniper) Close() {

	s.clock.Lock()
	defer s.clock.Unlock()

	var try int
loop:
	if s.isClose || try > 10 {
		return
	}

	if s.isdefer {

		s.mu.Lock()
		if len(s.ammoBag) == 0 && len(s.sendCache) == 0 {
			s.mu.Unlock()

			s.ammoBag = append(s.ammoBag, &protocol.Ammo{
				Id:   atomic.LoadUint32(&s.sendid),
				Kind: protocol.CLOSE,
				Body: nil,
			})

			s.writerBlocker.Close()

			select {

			case <-s.closeChan:

				s.writerBlocker.Close()
				s.acksign.Close()
				select {
				case <-s.readblock:
				default:
					close(s.readblock)
				}

			}
			s.isClose = true

		} else {
			s.mu.Unlock()
			time.Sleep(time.Second)
			try++
			goto loop

		}

	} else {
		s.ammoBagCach <- protocol.Ammo{
			Id:   atomic.LoadUint32(&s.sendid),
			Kind: protocol.CLOSE,
			Body: nil,
		}

		select {

		case <-s.closeChan:
			s.acksign.Close()
			close(s.readblock)
		}

		s.isClose = true
	}

}
