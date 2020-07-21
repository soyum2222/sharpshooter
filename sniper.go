package sharpshooter

import (
	"errors"
	"fmt"
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

const (
	DEFAULT_INIT_SENDWIND                      = 1024
	DEFAULT_INIT_RECEWIND                      = 1 << 10
	DEFAULT_INIT_PACKSIZE                      = 1024
	DEFAULT_INIT_HEALTHTICKER                  = 3
	DEFAULT_INIT_HEALTHCHECK_TIMEOUT_TRY_COUNT = 10
	DEFAULT_INIT_HANDSHACK_TIMEOUT             = 6
	DEFAULT_INIT_RTO_UNIT                      = float64(500 * time.Millisecond)
	DEFAULT_INIT_DELAY_ACK                     = float64(200 * time.Millisecond)
)

var (
	CLOSEERROR   = errors.New("the connection is closed")
	TIMEOUTERROR = errors.New("health monitor timeout ")
)

type Sniper struct {
	isDelay              bool
	isClose              bool
	noLeader             bool // it no has headquarters
	staSwitch            bool // Statistics switch
	maxWindows           int32
	oldWindows           int32
	healthTryCount       int32
	shootStatus          int32
	sendId               uint32
	beShotCurrentId      uint32
	currentWindowStartId uint32
	currentWindowEndId   uint32
	ackId                uint32
	index                uint32
	latestAckId          uint32
	lostCount            int32
	packageSize          int64

	//placeholder          int32 // in 32 bit OS must 8-byte alignment
	rtt            int64
	rto            int64
	timeFlag       int64
	totalFlow      int64 // statistics total flow used
	effectiveFlow  int64 // statistics effective flow
	sendCache      []byte
	writer         func(p []byte) (n int, err error)
	aim            *net.UDPAddr
	conn           *net.UDPConn
	timeoutTimer   *time.Timer
	healthTimer    *time.Timer
	ammoBag        []*protocol.Ammo
	beShotAmmoBag  []*protocol.Ammo
	receiveCache   []byte
	ammoBagCache   chan protocol.Ammo
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
		beShotAmmoBag: make([]*protocol.Ammo, DEFAULT_INIT_RECEWIND),
		handShakeSign: make(chan struct{}, 1),
		readBlock:     make(chan struct{}, 0),
		maxWindows:    DEFAULT_INIT_SENDWIND,
		mu:            sync.Mutex{},
		bemu:          sync.Mutex{},
		packageSize:   DEFAULT_INIT_PACKSIZE,
		ackSign:       block.NewBlocker(),
		writerBlocker: block.NewBlocker(),
		closeChan:     make(chan struct{}, 0),
		stopShotSign:  make(chan struct{}, 0),
		errorSign:     make(chan struct{}),
		sendCache:     make([]byte, 0),
		fecd:          newFecDecoder(10, 3),
		fece:          newFecEncoder(10, 3),
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
	s.beShotAmmoBag = make([]*protocol.Ammo, size)
}

func (s *Sniper) SetSendWin(size int32) {
	s.bemu.Lock()
	defer s.bemu.Unlock()
	s.maxWindows = size
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

		// send an ack
		id := atomic.LoadUint32(&s.ackId)

		if id != 0 {
			ammo := protocol.Ammo{
				Id:   id,
				Kind: protocol.ACK,
				Body: nil,
			}
			b := protocol.Marshal(ammo)
			_, _ = s.conn.WriteToUDP(b, s.aim)
			s.addTotalFlow(len(b))
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
				case <-s.errorSign:
				default:
					s.errorContainer.Store(errors.New(err.Error()))
					close(s.errorSign)
				}
			}

			s.addTotalFlow(len(b))

		}

		atomic.StoreUint32(&s.currentWindowEndId, currentWindowEndId)

		s.mu.Unlock()

		atomic.StoreInt32(&s.shootStatus, 0)

	}
	rto := atomic.LoadInt64(&s.rto)

	s.timeoutTimer.Reset(time.Duration(rto) * time.Nanosecond)
}

// remove already sent packages
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

// timed trigger send
func (s *Sniper) shooter() {

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

	_ = s.ackSign.Pass()

}

// timed trigger send ack
func (s *Sniper) ackSender() {

	timer := time.NewTimer(time.Duration(DEFAULT_INIT_DELAY_ACK))
	for {

		id := atomic.LoadUint32(&s.ackId)
		if id == 0 {

			err := s.ackSign.Block()
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
		fmt.Println("ack", id)

		_, err := s.conn.WriteToUDP(protocol.Marshal(ammo), s.aim)
		if err != nil {
			select {
			case <-s.errorSign:
				return
			default:
				s.errorContainer.Store(errors.New(err.Error()))
			}
		}

		//atomic.CompareAndSwapUint32(&s.ackId, id, 0)

		select {
		case <-timer.C:
		case <-s.closeChan:
			return
		}

		timer.Reset(time.Duration(s.rtt / 2))

	}
}

// receive ack
func (s *Sniper) score(id uint32) {
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
			if len(s.ammoBag) > 0 {
				s.mu.Unlock()
				s.shoot()
			} else {
				s.mu.Unlock()
			}

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
	//s.maxWindows -= int32(math.Abs(float64(s.maxWindows-s.oldWindows)) / 2)
	//s.oldWindows = old

	s.maxWindows -= 1
	//fmt.Println(s.maxWindows)

}

func (s *Sniper) expandWin() {

	s.maxWindows += 1
	//fmt.Println(s.maxWindows)
	//s.maxWindows += int32(math.Abs(float64(s.maxWindows-s.oldWindows)) / 2)
}

func (s *Sniper) beShot(ammo *protocol.Ammo) {

	fmt.Println(ammo.Id)
	//if ammo.Kind == protocol.OUTOFAMMO {
	//	atomic.StoreUint32(&s.ackId, 0)
	//	return
	//}

	s.bemu.Lock()
	defer s.bemu.Unlock()

	var flag bool

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

		if id == 0 && !flag {
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

	var anchor int
	for i := 0; i < len(s.beShotAmmoBag); {

		blocks := make([][]byte, s.fecd.dataShards+s.fecd.parShards)
		var empty int
		for j := 0; j < s.fecd.dataShards+s.fecd.parShards && i < len(s.beShotAmmoBag); j++ {

			if s.beShotAmmoBag[i] == nil {
				blocks[j] = nil
				empty++
			} else {
				blocks[j] = s.beShotAmmoBag[i].Body
			}

			i++
		}

		if empty > s.fecd.parShards {
			break
		}

		data, err := s.fecd.decode(blocks)
		if err == nil {

			anchor += s.fecd.dataShards + s.fecd.parShards

			s.receiveCache = append(s.receiveCache, data...)

			s.beShotCurrentId += uint32(s.fecd.dataShards + s.fecd.parShards)

		} else {
			break
		}
	}

	// cut off
	if anchor > len(s.beShotAmmoBag) {
		s.beShotAmmoBag = make([]*protocol.Ammo, DEFAULT_INIT_RECEWIND)
	} else {
		s.beShotAmmoBag = s.beShotAmmoBag[anchor:]
		s.beShotAmmoBag = append(s.beShotAmmoBag, make([]*protocol.Ammo, anchor)...)
	}

	var nid uint32
	for k, v := range s.beShotAmmoBag {

		if v == nil {
			flag = true
			nid = s.beShotCurrentId + uint32(k)
			break
		}
	}

	if nid == 0 && !flag {
		nid = s.beShotCurrentId + uint32(len(s.beShotAmmoBag))
	}

	s.ack(nid)

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

	n += copy(b, s.receiveCache)
	s.receiveCache = s.receiveCache[n:]

	//for len(s.beShotAmmoBag) != 0 && s.beShotAmmoBag[0] != nil {
	//
	//	cpn := copy(b[n:], s.beShotAmmoBag[0].Body)
	//
	//	n += cpn
	//
	//	if n == len(b) {
	//		if cpn < len(s.beShotAmmoBag[0].Body) {
	//			s.beShotAmmoBag[0].Body = s.beShotAmmoBag[0].Body[cpn:]
	//		} else {
	//			s.beShotAmmoBag = append(s.beShotAmmoBag[1:], nil)
	//			s.beShotCurrentId++
	//		}
	//		break
	//
	//	} else {
	//
	//		s.beShotAmmoBag = append(s.beShotAmmoBag[1:], nil)
	//		s.beShotCurrentId++
	//		continue
	//	}
	//
	//}

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
	remain := s.maxWindows - int32(len(s.ammoBag))

	if remain <= 0 {
		return
	}

	l := len(s.sendCache)

	if l == 0 {
		return
	}

	// anchor is mark sendCache current op index
	var anchor int64

	if int(l) < s.fece.dataShards*DEFAULT_INIT_PACKSIZE {
		anchor = int64(l)
	} else {
		anchor = int64(s.fece.dataShards * DEFAULT_INIT_PACKSIZE)
	}

	// TODO this block will not be GC , so maybe can be reused here , I will try
	body := s.sendCache[:anchor]

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

// send now,but this func is unavailable , should use delaySend
func (s *Sniper) write(b []byte) (n int, err error) {

	id := atomic.AddUint32(&s.sendId, 1)

	select {

	case s.ammoBagCache <- protocol.Ammo{
		Id:   id - 1,
		Kind: protocol.NORMAL,
		Body: b,
	}:
	case <-s.errorSign:
		return 0, s.errorContainer.Load().(error)

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
	s.writer = s.delaySend
	s.isDelay = true
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
	remain := int64(s.maxWindows)*int64(s.packageSize)*2 - int64(len(s.sendCache))

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
		//

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

	if s.isDelay {

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

	} else {
		s.ammoBagCache <- protocol.Ammo{
			Id:   atomic.LoadUint32(&s.sendId),
			Kind: protocol.CLOSE,
			Body: nil,
		}

		select {

		case <-s.closeChan:
			s.ackSign.Close()
			close(s.readBlock)
		}

		s.isClose = true
	}

}
