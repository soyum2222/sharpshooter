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
	DEFAULT_INIT_SENDCACH                      = 1024
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
	wrapFlag             int32
	packageSize          int
	cachsize             int
	timeout              int64
	shootTime            int64
	timeFlag             int64
	writer               func(p []byte) (n int, err error)
	aim                  *net.UDPAddr
	conn                 *net.UDPConn
	timeoutTimer         *time.Timer
	healthTimer          *time.Timer
	ammoBag              []*protocol.Ammo
	beShotAmmoBag        []*protocol.Ammo
	ammoBagCach          chan protocol.Ammo
	readblock            chan struct{}
	handshakesign        chan struct{}
	acksign              *block.Blocker
	writerBlocker        *block.Blocker
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
		cachsize:      DEFAULT_INIT_PACKSIZE * 1024 * 4,
		packageSize:   DEFAULT_INIT_PACKSIZE,
		acksign:       block.NewBlocker(),
		writerBlocker: block.NewBlocker(),
		ammoBagCach:   make(chan protocol.Ammo, DEFAULT_INIT_SENDCACH),
		closeChan:     make(chan struct{}, 0),
		loadsign:      make(chan struct{}, 1),
		stopshotsign:  make(chan struct{}, 0),
		errorchan:     make(chan error, 1),
	}
	sn.writer = sn.write
	return sn
}

func (s *Sniper) healthMonitor() {

	for {

		select {
		case <-s.healthTimer.C:
			if s.healthTryCount < DEFAULT_INIT_HEALTHCHECK_TIMEOUT_TRY_COUNT {

				fmt.Println("health")
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
				fmt.Println("time out")
				close(s.closeChan)

				return
			}

		case _, _ = <-s.closeChan:
			return
		}
	}

}

//func (s *Sniper) load() {
//
//	for {
//		//TODO closed
//		select {
//		case ammo := <-s.ammoBagCach:
//			s.mu.Lock()
//		loop:
//			if len(s.ammoBag) >= int(s.maxWindows)*2 {
//
//				if atomic.LoadInt32(&s.shootStatus) == 0 {
//					go s.shot()
//				}
//
//				s.mu.Unlock()
//				select {
//				case <-s.loadsign:
//				case <-s.closeChan:
//					return
//
//				}
//
//				s.mu.Lock()
//				goto loop
//
//			}
//
//			s.ammoBag = append(s.ammoBag, &ammo)
//
//			if atomic.LoadInt32(&s.shootStatus) == 0 {
//				//fmt.Println("load in shot")
//				go s.shot()
//			}
//			s.mu.Unlock()
//		case _, _ = <-s.closeChan:
//			return
//
//		}
//
//	}
//}

//func (s *Sniper) shot() {
//
//	timeout := s.timeout
//loop:
//
//	status := atomic.LoadInt32(&s.shootStatus)
//	if status&shooting >= 1 {
//		return
//	}
//
//	if !atomic.CompareAndSwapInt32(&s.shootStatus, status, status|shooting) {
//		//fmt.Println("bad lock")
//		//fmt.Println("out")
//		return
//	}
//
//	select {
//	case <-s.closeChan:
//		//fmt.Println("out1")
//		return
//	default:
//
//	}
//
//	select {
//	case s.stopshotsign <- struct{}{}:
//	default:
//	}
//
//	var c int
//l:
//	if len(s.ammoBag) < int(s.maxWindows) && c < 5 {
//		runtime.Gosched()
//		c++
//		goto l
//	}
//	c = 0
//
//	s.mu.Lock()
//	if s.ammoBag == nil || len(s.ammoBag) == 0 {
//
//		atomic.StoreInt32(&s.shootStatus, 0)
//
//		if s.timeoutticker != nil {
//			s.timeoutticker.Stop()
//		}
//
//		_, _ = s.conn.WriteToUDP(protocol.Marshal(protocol.Ammo{
//			Id:   0,
//			Kind: protocol.OUTOFAMMO,
//			Body: nil,
//		}), s.aim)
//
//		s.mu.Unlock()
//
//		//fmt.Println("out2")
//
//		return
//	}
//
//	for k, _ := range s.ammoBag {
//
//		if s.ammoBag[k] == nil {
//			continue
//		}
//
//		if k > int(s.maxWindows) {
//			break
//		}
//
//		s.currentWindowEndId = s.currentWindowStartId + uint32(k)
//
//		_, err := s.conn.WriteToUDP(protocol.Marshal(*s.ammoBag[k]), s.aim)
//
//		//fmt.Println("send msg ---", string(s.ammoBag[k].Body))
//
//		if err != nil {
//			panic(err)
//		}
//
//	}
//
//	for {
//		status = atomic.LoadInt32(&s.shootStatus)
//		if atomic.CompareAndSwapInt32(&s.shootStatus, status, (status&(^shooting))|waittimeout) {
//			break
//		}
//	}
//
//	s.mu.Unlock()
//
//	// todo leak
//	s.timeoutticker = time.NewTicker(time.Duration(timeout) * time.Nanosecond)
//
//	select {
//
//	case <-s.timeoutticker.C:
//		//fmt.Println("timeout", s.timeout)
//		timeout = timeout * 2
//
//		if s.maxWindows > 1 {
//			atomic.StoreInt32(&s.maxWindows, int32(float64(s.maxWindows)/1.25))
//		}
//
//		fmt.Println(s.maxWindows)
//
//		for {
//			status = atomic.LoadInt32(&s.shootStatus)
//			if atomic.CompareAndSwapInt32(&s.shootStatus, status, (status & (^waittimeout))) {
//				break
//			}
//		}
//		goto loop
//
//	case <-s.stopshotsign:
//		fmt.Println(s.maxWindows)
//
//		//fmt.Println("out3")
//		atomic.AddInt32(&s.maxWindows, 10)
//		//fmt.Println(s.maxWindows)
//
//		return
//
//	}
//
//}

func (s *Sniper) shoot() {
	if atomic.CompareAndSwapInt32(&s.shootStatus, 0, shooting) {
		s.mu.RLock()

		for k, _ := range s.ammoBag {

			if s.ammoBag[k] == nil {
				continue
			}

			if k > int(s.maxWindows) {
				break
			}

			s.currentWindowEndId = s.currentWindowStartId + uint32(k)

			_, err := s.conn.WriteToUDP(protocol.Marshal(*s.ammoBag[k]), s.aim)

			if err != nil {
				panic(err)
			}

		}
		s.mu.RUnlock()

		atomic.StoreInt32(&s.shootStatus, 0)
	}

	s.timeoutTimer.Reset(time.Duration(s.timeout) * time.Nanosecond)
}

func (s *Sniper) shooter() {
	s.timeoutTimer = time.NewTimer(time.Duration(s.timeout) * time.Nanosecond)
	for {

		select {

		case <-s.timeoutTimer.C:

			s.timeout = int64(float64(s.timeout) * 1.5)

			atomic.StoreInt32(&s.maxWindows, int32(float64(s.maxWindows)/1.25))

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

	timer := time.NewTimer(time.Millisecond * 20)
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
			panic(err)
		}

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

	if index == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	//if index > int(s.maxWindows) {
	//	s.ammoBag = s.ammoBag[index:]
	//	atomic.AddUint32(&s.currentWindowStartId, uint32(index))
	//	select {
	//	case s.loadsign <- struct{}{}:
	//	default:
	//	}
	//	return
	//}

	if index >= len(s.ammoBag) {
		atomic.AddUint32(&s.currentWindowStartId, uint32(index))
		_ = s.writerBlocker.Pass()
		return
	}

	ackAttempts := s.ammoBag[index].AckAdd()
	if len(s.ammoBag) > index && ackAttempts > 3 && s.currentWindowStartId != s.currentWindowEndId {
		_, err := s.conn.WriteToUDP(protocol.Marshal(*s.ammoBag[index]), s.aim)
		if err != nil {
			panic(err)
		}
		if ackAttempts > 6 {
			atomic.StoreInt32(&s.maxWindows, int32(float64(s.maxWindows)/1.5))
		}

		return
	}

	s.ammoBag = s.ammoBag[index:]

	atomic.AddUint32(&s.currentWindowStartId, uint32(index))

	_ = s.writerBlocker.Pass()

	if s.currentWindowStartId >= s.currentWindowEndId {
		s.maxWindows += 10
		fmt.Println(s.maxWindows)
	}

}

func (s *Sniper) beShot(ammo *protocol.Ammo) {

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

		case _, _ = <-s.closeChan:
			return 0, CLOSEERROR

		case err := <-s.errorchan:
			return 0, err

		}

	}

	return
}

//func (s *Sniper) wrap() {
//
//	var skipcount int
//
//	s.bemu.Lock()
//	defer s.bemu.Unlock()
//
//	for {
//		select {
//		case <-s.closeChan:
//			return
//		default:
//
//		}
//
//		if len(s.deferSendQueue) >= s.packageSize {
//
//			skipcount = 0
//
//			id := atomic.AddUint32(&s.sendid, 1)
//
//			select {
//			case <-s.closeChan:
//				return
//
//			case s.ammoBagCach <- protocol.Ammo{
//				Id:   id - 1,
//				Kind: protocol.NORMAL,
//				Body: s.deferSendQueue[:s.packageSize],
//			}:
//
//			}
//
//			s.deferSendQueue = s.deferSendQueue[s.packageSize:]
//
//		} else if len(s.deferSendQueue) == 0 {
//			return
//
//		} else {
//
//			if skipcount < 3 {
//				skipcount++
//				time.Sleep(time.Millisecond * 50)
//				continue
//			} else {
//				skipcount = 0
//
//				id := atomic.AddUint32(&s.sendid, 1)
//
//				select {
//				case <-s.closeChan:
//					return
//
//				case s.ammoBagCach <- protocol.Ammo{
//					Id:   id - 1,
//					Kind: protocol.NORMAL,
//					Body: s.deferSendQueue,
//				}:
//				}
//
//				s.deferSendQueue = s.deferSendQueue[:0]
//			}
//
//		}
//	}
//
//}

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
}

func (s *Sniper) deferSend(b []byte) (n int, err error) {

	n = len(b)

	// It's acceptable to compete here
	if len(s.ammoBag) >= int(s.maxWindows) {

		//s.maxWindows += 10
		//fmt.Println(s.maxWindows)

		s.shoot()
	}

	s.mu.Lock()

	// If possible , it is better to the data append on previous ammo
	ammoLength := len(s.ammoBag)

	if ammoLength > 0 {
		// If previous ammo is not full ,then append it
		ammo := s.ammoBag[ammoLength-1]

		remain := s.packageSize - len(ammo.Body)

		// remain > b.len , put of all b to ammo
		if remain > n {
			ammo.Body = append(ammo.Body, b...)
			b = nil
			return n, nil
		} else {
			ammo.Body = append(ammo.Body, b[:remain]...)
			b = b[remain:]
		}
	}

loop:
	id := atomic.AddUint32(&s.sendid, 1)
	// But here no
	if len(b) > s.packageSize {

		s.ammoBag = append(s.ammoBag, &protocol.Ammo{
			Id:   id - 1,
			Kind: protocol.NORMAL,
			Body: b[:s.packageSize],
		})

		b = b[s.packageSize:]

		goto loop

	}

	s.ammoBag = append(s.ammoBag, &protocol.Ammo{
		Id:   id - 1,
		Kind: protocol.NORMAL,
		Body: append([]byte{}, b...),
	})

	var needblock bool

	if len(s.ammoBag) > int(s.maxWindows) {
		needblock = true
	}

	s.mu.Unlock()

	if needblock {
		s.writerBlocker.Block()
	}

	return

}

func (s *Sniper) Close() {

	if s.isClose {
		return
	}

	if s.isdefer {

		//var try int
	loop:
		//try++
		//
		//if try == 10 {
		//
		//	s.ammoBagCach <- protocol.Ammo{
		//		Id:   atomic.LoadUint32(&s.sendid),
		//		Kind: protocol.CLOSE,
		//		Body: nil,
		//	}
		//
		//	select {
		//	case <-s.closeChan:
		//		s.writerBlocker.Close()
		//		s.acksign.Close()
		//		select {
		//		case <-s.readblock:
		//		default:
		//			close(s.readblock)
		//		}
		//	}
		//
		//	s.isClose = true
		//}

		s.mu.Lock()
		if len(s.ammoBag) == 0 {
			s.mu.Unlock()

			s.ammoBag = append(s.ammoBag, &protocol.Ammo{
				Id:   atomic.LoadUint32(&s.sendid),
				Kind: protocol.CLOSE,
				Body: nil,
			})

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
