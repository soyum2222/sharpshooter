package core

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

var (
	CLOSEERROR = errors.New("the connection is closed")
)

type Sniper struct {
	aim                  *net.UDPAddr
	conn                 *net.UDPConn
	ammoBag              []*protocol.Ammo
	ammoBagCach          chan protocol.Ammo
	cachsize             int
	beShotAmmoBag        []*protocol.Ammo
	maxWindows           int
	packageSize          int
	sendid               uint32
	beShotCurrentId      uint32
	currentWindowStartId uint32
	currentWindowEndId   uint32
	writer               func(p []byte) (n int, err error)
	isdefer              bool
	timeout              int64
	timeoutPanicTimer    *time.Timer
	shootTime            int64
	shootStatus          int32
	deferSendQueue       []byte
	deferBlocker         block.Blocker
	timeoutticker        *time.Ticker
	mu                   sync.RWMutex
	bemu                 sync.Mutex
	dqmu                 sync.Mutex
	ackId                uint32
	handshakesign        chan struct{}
	readblock            chan struct{}
	acksign              chan struct{}
	closeChan            chan struct{}
	loadsign             chan struct{}
	stopshotsign         chan struct{}
	errorchan            chan error
	isClose              bool
}

func NewSniper(conn *net.UDPConn, aim *net.UDPAddr, timeout int64) *Sniper {
	sn := &Sniper{
		aim:           aim,
		conn:          conn,
		timeout:       timeout,
		beShotAmmoBag: make([]*protocol.Ammo, 100),
		handshakesign: make(chan struct{}, 1),
		readblock:     make(chan struct{}, 0),
		maxWindows:    200,
		mu:            sync.RWMutex{},
		bemu:          sync.Mutex{},
		cachsize:      100,
		packageSize:   1000,
		acksign:       make(chan struct{}, 0),
		ammoBagCach:   make(chan protocol.Ammo, 100),
		closeChan:     make(chan struct{}, 0),
		loadsign:      make(chan struct{}, 1),
		stopshotsign:  make(chan struct{}, 0),
		errorchan:     make(chan error, 1),
	}
	sn.writer = sn.write
	sn.cachsize = sn.packageSize * 100
	go sn.load()
	go sn.heartbeat()
	return sn

}

func (s *Sniper) heartbeat() {

	t := time.NewTicker(time.Second)
	for {

		select {
		case <-t.C:

			if atomic.LoadInt32(&s.shootStatus) != 0 {
				return
			}

			ammo := protocol.Ammo{
				Id:   0,
				Kind: protocol.Heartbeat,
				Body: nil,
			}

			s.conn.WriteToUDP(protocol.Marshal(ammo), s.aim)
		}

	}

}

func (s *Sniper) timeoutPanic() {

	if s.timeoutPanicTimer == nil {
		s.timeoutPanicTimer = time.NewTimer(time.Second * 10)
	}

	select {
	case <-s.timeoutPanicTimer.C:
		s.timeoutPanicTimer.Stop()
		s.errorchan <- errors.New("timeout")
	}

}

func (s *Sniper) load() {

	for {

		ammo, ok := <-s.ammoBagCach
		if !ok {
			return
		}
		s.mu.Lock()
	loop:
		if len(s.ammoBag) >= s.maxWindows {

			if atomic.LoadInt32(&s.shootStatus)&(shooting|waittimeout) == 0 {
				go s.Shot()
			}

			s.mu.Unlock()
			<-s.loadsign

			s.mu.Lock()
			goto loop

		}

		s.ammoBag = append(s.ammoBag, &ammo)

		if atomic.LoadInt32(&s.shootStatus)&(shooting|waittimeout) == 0 {
			go s.Shot()
		}
		s.mu.Unlock()

	}
}

func (s *Sniper) Shot() {

	fmt.Println("in")
loop:

	status := atomic.LoadInt32(&s.shootStatus)
	if status&shooting >= 1 {
		return
	}

	if !atomic.CompareAndSwapInt32(&s.shootStatus, status, status|shooting) {
		//fmt.Println("bad lock")
		fmt.Println("out")
		return
	}

	select {
	case <-s.closeChan:
		fmt.Println("out1")
		return
	default:

	}

	if s.timeout < int64(10*time.Millisecond) {
		s.timeout = int64(10 * time.Millisecond)
	}

	// todo leak
	s.timeoutticker = time.NewTicker(time.Duration(s.timeout))

	select {
	case s.stopshotsign <- struct{}{}:
	default:
	}

	s.mu.Lock()
	if s.ammoBag == nil || len(s.ammoBag) == 0 {
		atomic.StoreInt32(&s.shootStatus, 0)
		s.timeoutticker.Stop()
		s.mu.Unlock()

		go s.heartbeat()
		fmt.Println("out2")
		return
	}

	for k, _ := range s.ammoBag {

		if s.ammoBag[k] == nil {
			continue
		}

		if k > s.maxWindows {
			break
		}

		s.currentWindowEndId = s.currentWindowStartId + uint32(k)

		_, err := s.conn.WriteToUDP(protocol.Marshal(*s.ammoBag[k]), s.aim)

		//fmt.Println(len(s.ammoBag[k].Body))
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

	select {

	case <-s.timeoutticker.C:
		fmt.Println("timeout", s.timeout)

		if s.timeout < int64(time.Second*10) {
			s.timeout += int64(time.Millisecond * 10)
		}
		if s.maxWindows > 0 {
			s.maxWindows -= 10
		}

		for {
			status = atomic.LoadInt32(&s.shootStatus)
			if atomic.CompareAndSwapInt32(&s.shootStatus, status, (status & (^waittimeout))) {
				break
			}
		}
		goto loop

	case <-s.stopshotsign:
		fmt.Println("sub timeout")
		if s.timeout > int64(time.Millisecond*10) {
			s.timeout -= int64(time.Millisecond * 10)
		}
		s.maxWindows += 10

		fmt.Println("out3")
		return

	}

}

func (s *Sniper) ack(id uint32) {

	cid := atomic.LoadUint32(&s.ackId)
	if cid < id {
		atomic.CompareAndSwapUint32(&s.ackId, cid, id)
	}

	select {
	case s.acksign <- struct{}{}:
	default:
	}

}

func (s *Sniper) ackSender() {

	timer := time.NewTimer(time.Duration(s.timeout / 10))
	for {

		id := atomic.LoadUint32(&s.ackId)
		if id == 0 {

			_, ok := <-s.acksign
			if !ok {
				return
			}

		}

		//fmt.Println("nid", id)
		ammo := protocol.Ammo{
			Id:   id,
			Kind: protocol.ACK,
			Body: nil,
		}
		_, err := s.conn.WriteToUDP(protocol.Marshal(ammo), s.aim)
		if err != nil {
			panic(err)
		}

		atomic.CompareAndSwapUint32(&s.ackId, id, 0)

		select {
		case <-timer.C:
		case <-s.closeChan:
			return
		}

		timer.Reset(time.Duration(s.timeout / 10))

	}
}

func (s *Sniper) score(id uint32) {

	if id < atomic.LoadUint32(&s.currentWindowStartId) {
		return
	}

	index := int(id) - int(atomic.LoadUint32(&s.currentWindowStartId))

	if index > len(s.ammoBag) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if index > s.maxWindows {
		s.ammoBag = s.ammoBag[index:]
		atomic.AddUint32(&s.currentWindowStartId, uint32(index))
		select {
		case s.loadsign <- struct{}{}:
		default:
		}
		return
	}

	if len(s.ammoBag) > index && s.ammoBag[index].AckAdd() > 3 && s.currentWindowStartId != s.currentWindowEndId {
		//fmt.Println("fast reshot")
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
			go s.Shot()
		}

	}

}

func (s *Sniper) BeShot(ammo *protocol.Ammo) {

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

	select {

	case s.readblock <- struct{}{}:

	default:

	}

	s.ack(nid)
}

func (s *Sniper) Read(b []byte) (n int, err error) {

loop:

	s.bemu.Lock()
	for len(s.beShotAmmoBag) != 0 && s.beShotAmmoBag[0] != nil {

		if s.beShotAmmoBag[0] == nil {
			break
		}

		n += copy(b[n:], s.beShotAmmoBag[0].Body)

		if n == len(b) {
			if n < len(s.beShotAmmoBag[0].Body) {
				s.beShotAmmoBag[0].Body = s.beShotAmmoBag[0].Body[n:]
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

		if len(s.deferSendQueue) >= s.packageSize {

			skipcount = 0

			id := atomic.AddUint32(&s.sendid, 1)

			s.dqmu.Lock()
			s.ammoBagCach <- protocol.Ammo{
				Id:   id - 1,
				Kind: protocol.NORMAL,
				Body: s.deferSendQueue[:s.packageSize],
			}

			s.deferSendQueue = s.deferSendQueue[s.packageSize:]
			s.dqmu.Unlock()

			s.deferBlocker.Pass()

		} else if len(s.deferSendQueue) == 0 {
			// give way
			runtime.Gosched()
		} else {
			if skipcount < 3 {
				skipcount++
				time.Sleep(time.Millisecond * 50)
				continue
			} else {
				skipcount = 0

				id := atomic.AddUint32(&s.sendid, 1)

				s.dqmu.Lock()
				s.ammoBagCach <- protocol.Ammo{
					Id:   id - 1,
					Kind: protocol.NORMAL,
					Body: s.deferSendQueue,
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

		if len(s.deferSendQueue)+len(b) > s.cachsize {
			// need block
			s.deferBlocker.Block()
			goto loop

		} else {
			s.dqmu.Lock()
			s.deferSendQueue = append(s.deferSendQueue, b...)
			s.dqmu.Unlock()
			return len(b), nil
		}
	}

}

func (s *Sniper) Close() error {

	t := time.NewTicker(time.Second * 10)
	if s.isdefer {

	loop:
		if len(s.deferSendQueue) == 0 {
			s.ammoBagCach <- protocol.Ammo{
				Id:   atomic.LoadUint32(&s.sendid),
				Kind: protocol.CLOSE,
				Body: nil,
			}
			select {
			case <-s.closeChan:
			case <-t.C:
				t.Stop()
			}

		} else {
			runtime.Gosched()
			goto loop

		}

	} else {

		s.isClose = true
		s.ammoBagCach <- protocol.Ammo{
			Id:   atomic.LoadUint32(&s.sendid),
			Kind: protocol.CLOSE,
			Body: nil,
		}

		select {
		case <-s.closeChan:
		case <-t.C:
			t.Stop()
		}
	}

	return s.conn.Close()
}
