package sharpshooter

import (
	"errors"
	"fmt"
	"net"
	"runtime"
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

const (
	DEFAULT_INIT_SENDWIND                      = 32
	DEFAULT_INIT_RECEWIND                      = 1024
	DEFAULT_INIT_PACKSIZE                      = 1024
	DEFAULT_INIT_HEALTHTICKER                  = 3
	DEFAULT_INIT_HEALTHCHECK_TIMEOUT_TRY_COUNT = 3
	DEFAULT_INIT_SENDCACH                      = 0xff
	DEFAULT_INIT_MIN_TIMEOUT                   = int64(1000 * time.Millisecond)
)

var (
	CLOSEERROR   = errors.New("the connection is closed")
	TIMEOUTERROR = errors.New("health monitor timeout ")
)

type Sniper struct {
	isdefer              bool
	isClose              bool
	maxWindows           int32
	healthTryCount       int32
	sendid               uint32
	beShotCurrentId      uint32
	currentWindowStartId uint32
	currentWindowEndId   uint32
	shootStatus          int32
	ackId                uint32
	packageSize          int
	cachsize             int
	timeout              int64
	shootTime            int64
	timeFlag             int64
	writer               func(p []byte) (n int, err error)
	aim                  *net.UDPAddr
	conn                 *net.UDPConn
	timeoutticker        *time.Ticker
	healthTimer          *time.Timer
	deferSendQueue       []byte
	ammoBag              []*protocol.Ammo
	beShotAmmoBag        []*protocol.Ammo
	ammoBagCach          chan protocol.Ammo
	readblock            chan struct{}
	handshakesign        chan struct{}
	acksign              *block.Blocker
	deferBlocker         *block.Blocker
	closeChan            chan struct{}
	loadsign             chan struct{}
	stopshotsign         chan struct{}
	errorchan            chan error
	mu                   sync.RWMutex
	bemu                 sync.Mutex
	dqmu                 sync.RWMutex
}

func NewSniper(conn *net.UDPConn, aim *net.UDPAddr, timeout int64) *Sniper {

	sn := &Sniper{
		aim:           aim,
		conn:          conn,
		timeout:       timeout,
		beShotAmmoBag: make([]*protocol.Ammo, DEFAULT_INIT_RECEWIND),
		handshakesign: make(chan struct{}, 1),
		readblock:     make(chan struct{}, 0),
		maxWindows:    DEFAULT_INIT_SENDWIND,
		mu:            sync.RWMutex{},
		bemu:          sync.Mutex{},
		cachsize:      DEFAULT_INIT_PACKSIZE * 10,
		packageSize:   DEFAULT_INIT_PACKSIZE,
		acksign:       block.NewBlocker(),
		deferBlocker:  block.NewBlocker(),
		ammoBagCach:   make(chan protocol.Ammo, DEFAULT_INIT_SENDCACH),
		closeChan:     make(chan struct{}, 0),
		loadsign:      make(chan struct{}, 1),
		stopshotsign:  make(chan struct{}, 0),
		errorchan:     make(chan error, 1),
	}

	sn.writer = sn.write
	go sn.load()
	return sn
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
				s.errorchan <- TIMEOUTERROR
				s.acksign.Close()
				close(s.readblock)
				close(s.closeChan)

				return
			}

		case <-s.closeChan:
			return
		}
	}

}

func (s *Sniper) load() {

	for {
		//TODO closed
		ammo, ok := <-s.ammoBagCach
		if !ok {
			return
		}
		s.mu.Lock()
	loop:
		if len(s.ammoBag) >= int(s.maxWindows)*2 {

			if atomic.LoadInt32(&s.shootStatus) == 0 {
				go s.shot()
			}

			s.mu.Unlock()
			select {
			case <-s.loadsign:
			case <-s.closeChan:
				return

			}

			s.mu.Lock()
			goto loop

		}

		s.ammoBag = append(s.ammoBag, &ammo)

		if atomic.LoadInt32(&s.shootStatus) == 0 {
			//fmt.Println("load in shot")
			go s.shot()
		}
		s.mu.Unlock()

	}
}

func (s *Sniper) shot() {

	timeout := s.timeout
loop:

	status := atomic.LoadInt32(&s.shootStatus)
	if status&shooting >= 1 {
		return
	}

	if !atomic.CompareAndSwapInt32(&s.shootStatus, status, status|shooting) {
		//fmt.Println("bad lock")
		//fmt.Println("out")
		return
	}

	select {
	case <-s.closeChan:
		//fmt.Println("out1")
		return
	default:

	}

	select {
	case s.stopshotsign <- struct{}{}:
	default:
	}

	var c int
l:
	if len(s.ammoBag) < int(s.maxWindows) && c < 5 {
		runtime.Gosched()
		c++
		goto l
	}
	c = 0

	s.mu.Lock()
	if s.ammoBag == nil || len(s.ammoBag) == 0 {

		atomic.StoreInt32(&s.shootStatus, 0)

		s.timeoutticker.Stop()

		_, _ = s.conn.WriteToUDP(protocol.Marshal(protocol.Ammo{
			Id:   0,
			Kind: protocol.OUTOFAMMO,
			Body: nil,
		}), s.aim)

		s.mu.Unlock()

		//fmt.Println("out2")

		return
	}

	for k, _ := range s.ammoBag {

		if s.ammoBag[k] == nil {
			continue
		}

		if k > int(s.maxWindows) {
			break
		}

		s.currentWindowEndId = s.currentWindowStartId + uint32(k)

		_, err := s.conn.WriteToUDP(protocol.Marshal(*s.ammoBag[k]), s.aim)

		//fmt.Println("send msg ---", string(s.ammoBag[k].Body))

		if err != nil {
			panic(err)
		}

	}

	for {
		status = atomic.LoadInt32(&s.shootStatus)
		if atomic.CompareAndSwapInt32(&s.shootStatus, status, (status&(^shooting))|waittimeout) {
			break
		}
	}

	s.mu.Unlock()

	// todo leak
	s.timeoutticker = time.NewTicker(time.Duration(timeout) * time.Nanosecond)

	select {

	case <-s.timeoutticker.C:
		//fmt.Println("timeout", s.timeout)
		timeout = timeout * 2

		if s.maxWindows > 1 {
			atomic.StoreInt32(&s.maxWindows, int32(float64(s.maxWindows)/1.25))
		}

		fmt.Println(s.maxWindows)

		for {
			status = atomic.LoadInt32(&s.shootStatus)
			if atomic.CompareAndSwapInt32(&s.shootStatus, status, (status & (^waittimeout))) {
				break
			}
		}
		goto loop

	case <-s.stopshotsign:

		//fmt.Println("out3")
		atomic.AddInt32(&s.maxWindows, 10)
		//fmt.Println(s.maxWindows)

		return

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

	timer := time.NewTimer(time.Millisecond * 200)
	for {

		id := atomic.LoadUint32(&s.ackId)
		if id == 0 {

			err := s.acksign.Block()
			if err != nil {
				return
			}
			continue

		}

		//fmt.Println("ack id", id)
		//fmt.Println("current id", s.beShotCurrentId)
		ammo := protocol.Ammo{
			Id:   id,
			Kind: protocol.ACK,
			Body: nil,
		}

		_, err := s.conn.WriteToUDP(protocol.Marshal(ammo), s.aim)
		if err != nil {
			panic(err)
		}

		//atomic.CompareAndSwapUint32(&s.ackId, id, 0)

		select {
		case <-timer.C:
		case <-s.closeChan:
			return
		}

		timer.Reset(time.Millisecond * 200)

	}
}

func (s *Sniper) score(id uint32) {

	//fmt.Println("id ", id)
	//fmt.Println("current id", s.currentWindowStartId)
	//fmt.Println("end id", s.currentWindowEndId)
	if id < atomic.LoadUint32(&s.currentWindowStartId) {
		return
	}

	index := int(id) - int(atomic.LoadUint32(&s.currentWindowStartId))

	if index > len(s.ammoBag) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if index > int(s.maxWindows) {
		s.ammoBag = s.ammoBag[index:]
		atomic.AddUint32(&s.currentWindowStartId, uint32(index))
		select {
		case s.loadsign <- struct{}{}:
		default:
		}
		return
	}

	if len(s.ammoBag) > index && s.ammoBag[index].AckAdd() > 3 && s.currentWindowStartId != s.currentWindowEndId {
		_, err := s.conn.WriteToUDP(protocol.Marshal(*s.ammoBag[index]), s.aim)
		if err != nil {
			panic(err)
		}
	}

	s.ammoBag = s.ammoBag[index:]

	select {
	case s.loadsign <- struct{}{}:
	default:
	}

	atomic.AddUint32(&s.currentWindowStartId, uint32(index))

	if s.currentWindowStartId >= s.currentWindowEndId {
		if atomic.LoadInt32(&s.shootStatus)&(shooting) == 0 {
			go s.shot()
		}

	}

}

func (s *Sniper) beShot(ammo *protocol.Ammo) {

	//fmt.Println("receive id ", ammo.Id)
	//fmt.Println("receive msg --", string(ammo.Body))

	if ammo.Kind == protocol.OUTOFAMMO {
		atomic.StoreUint32(&s.ackId, 0)
		return
	}

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

		if cpn == len(b) {
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

		case <-s.closeChan:
			return 0, CLOSEERROR

		case err := <-s.errorchan:
			return 0, err

		}

	}

	return
}

func (s *Sniper) wrap() {

	var skipcount int
	for {
		select {
		case <-s.closeChan:
			return
		default:

		}

		s.dqmu.RLock()
		if len(s.deferSendQueue) >= s.packageSize {
			s.dqmu.RUnlock()

			skipcount = 0

			id := atomic.AddUint32(&s.sendid, 1)

			s.dqmu.Lock()

			select {
			case <-s.closeChan:
				s.dqmu.Unlock()
				return
			default:

			}
			s.ammoBagCach <- protocol.Ammo{
				Id:   id - 1,
				Kind: protocol.NORMAL,
				Body: s.deferSendQueue[:s.packageSize],
			}

			s.deferSendQueue = s.deferSendQueue[s.packageSize:]
			s.dqmu.Unlock()

			s.deferBlocker.Pass()

		} else if len(s.deferSendQueue) == 0 {
			s.dqmu.RUnlock()

			s.deferBlocker.PassB()
			// give way
			//time.Sleep(time.Millisecond * 500)
			runtime.Gosched()
		} else {
			s.dqmu.RUnlock()

			if skipcount < 3 {
				skipcount++
				time.Sleep(time.Millisecond * 50)
				continue
			} else {
				skipcount = 0

				id := atomic.AddUint32(&s.sendid, 1)

				s.dqmu.Lock()
				select {
				case <-s.closeChan:
					s.dqmu.Unlock()
					return
				case s.ammoBagCach <- protocol.Ammo{
					Id:   id - 1,
					Kind: protocol.NORMAL,
					Body: s.deferSendQueue,
				}:
				}

				s.deferSendQueue = s.deferSendQueue[:0]
				s.dqmu.Unlock()
				s.deferBlocker.Pass()
			}

		}
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
	case err := <-s.errorchan:
		return 0, err

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
	go s.wrap()
}

func (s *Sniper) deferSend(b []byte) (n int, err error) {

loop:
	select {
	case <-s.closeChan:
		return 0, CLOSEERROR
	case err := <-s.errorchan:
		return 0, err

	default:

		s.dqmu.RLock()
		if len(s.deferSendQueue)+len(b) > s.cachsize {

			if len(s.deferSendQueue) > s.cachsize {
				s.dqmu.RUnlock()
				// need block
				err = s.deferBlocker.Block()
				if err != nil {
					return 0, err
				}
				goto loop

			}
			vacancy := s.cachsize - len(s.deferSendQueue)

			s.dqmu.RUnlock()
			s.dqmu.Lock()

			s.deferSendQueue = append(s.deferSendQueue, b[:vacancy]...)
			b = b[vacancy:]

			s.dqmu.Unlock()

			goto loop

		} else {
			s.dqmu.RUnlock()
			s.dqmu.Lock()
			s.deferSendQueue = append(s.deferSendQueue, b...)
			s.dqmu.Unlock()
			return len(b), nil
		}
	}

}

func (s *Sniper) Close() {

	if s.isClose {
		return
	}

	if s.isdefer {

	loop:
		s.mu.Lock()
		if len(s.ammoBag) == 0 && len(s.deferSendQueue) == 0 {
			s.mu.Unlock()

			s.ammoBagCach <- protocol.Ammo{
				Id:   atomic.LoadUint32(&s.sendid),
				Kind: protocol.CLOSE,
				Body: nil,
			}

			select {

			case <-s.closeChan:

				s.deferBlocker.Close()
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
			runtime.Gosched()
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
