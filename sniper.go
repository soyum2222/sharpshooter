package sharpshooter

import (
	"errors"
	"fmt"
	"net"
	"runtime"
	"sharpshooter/protocol"
	"sharpshooter/tool"
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
	DEFAULT_INIT_SENDWIND                      = 128
	DEFAULT_INIT_MAXSENDWIND                   = 1024
	DEFAULT_INIT_RECEWIND                      = 1 << 20
	DEFAULT_INIT_PACKSIZE                      = 1024
	DEFAULT_INIT_HEALTHTICKER                  = 3
	DEFAULT_INIT_HEALTHCHECK_TIMEOUT_TRY_COUNT = 3
	DEFAULT_INIT_SENDCACH                      = 1 << 20
	DEFAULT_INIT_MIN_TIMEOUT                   = int64(time.Millisecond)
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
	index                uint32
	latestAckId          uint32
	packageSize          int
	lostCount            int
	placeholder          int32 // in 32 bit OS must 8-byte alignment
	rtt                  int64
	rto                  int64
	timeFlag             int64
	sendCacheSize        int64
	sendCache            []byte
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
	stopshotsign         chan struct{}
	errorchan            chan error
	mu                   sync.RWMutex
	bemu                 sync.Mutex
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
		errorchan:     make(chan error, 1),
		sendCache:     make([]byte, 0),
		sendCacheSize: DEFAULT_INIT_SENDCACH,
	}
	sn.writer = sn.write
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
				s.writerBlocker.Close()
				fmt.Println("time out")
				close(s.closeChan)

				return
			}

		case _, _ = <-s.closeChan:
			return
		}
	}

}

func (s *Sniper) shoot() {
	if atomic.CompareAndSwapInt32(&s.shootStatus, 0, shooting) {
		defer tool.TimeConsuming()()

		fmt.Println("max windwos", s.maxWindows)
		s.mu.Lock()

		s.flush()

		s.wrap()

		var currentWindowEndId uint32

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
				panic(err)
			}

			Flow += len(b)

		}

		atomic.StoreUint32(&s.currentWindowEndId, currentWindowEndId)

		s.mu.Unlock()

		atomic.StoreInt32(&s.shootStatus, 0)
	}

	s.timeoutTimer.Reset(time.Duration(s.rto) * time.Nanosecond)
}

func (s *Sniper) flush() {
	defer tool.TimeConsuming()()
	if len(s.ammoBag) < int(s.index) || s.index == 0 {
		return
	}
	fmt.Println("index:", s.index)
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

	fmt.Println("rto ", s.rto)
	s.timeoutTimer = time.NewTimer(time.Duration(s.rto) * time.Nanosecond)
	for {

		select {

		case <-s.timeoutTimer.C:

			s.rto = int64(float64(s.rto) * 2)
			if s.maxWindows > 2 {
				atomic.StoreInt32(&s.maxWindows, (s.maxWindows)/2)
			}

			break

		case <-s.closeChan:
			return
		}

		fmt.Println("timeout shoot")
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

	fmt.Println("rtt", s.rtt)
	timer := time.NewTimer(time.Duration(s.rtt) * time.Nanosecond)
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
	fmt.Println("ack id", id)

	s.mu.Lock()

	if id < atomic.LoadUint32(&s.currentWindowStartId) {
		s.mu.Unlock()
		return
	}

	index := int(id) - int(atomic.LoadUint32(&s.currentWindowStartId))

	if index > len(s.ammoBag) {
		s.mu.Unlock()
		return
		//index = len(s.ammoBag)
	}

	atomic.StoreUint32(&s.index, uint32(index))

	if id == s.latestAckId {
		s.lostCount++

		if s.lostCount > 10 {

			fmt.Println("fast")

			if index >= len(s.ammoBag) {
				_ = s.writerBlocker.Pass()
				_, _ = s.conn.WriteTo(protocol.Marshal(protocol.Ammo{
					Length: 0,
					Id:     0,
					Kind:   protocol.OUTOFAMMO,
					Body:   nil,
				}), s.aim)
				s.mu.Unlock()
				return
			}

			_, _ = s.conn.WriteToUDP(protocol.Marshal(*s.ammoBag[index]), s.aim)

		}

	} else {
		s.latestAckId = id
	}

	// now send window is send clean
	if id >= atomic.LoadUint32(&s.currentWindowEndId) {

		atomic.StoreInt64(&s.rto, atomic.LoadInt64(&s.rtt)*2)
		if s.maxWindows < DEFAULT_INIT_MAXSENDWIND {
			s.maxWindows = s.maxWindows * 2
		} else {
			s.maxWindows = s.maxWindows + 1
		}
		_ = s.writerBlocker.Pass()
		s.mu.Unlock()

		s.shoot()

	} else {
		s.mu.Unlock()
	}

	return

}

func (s *Sniper) beShot(ammo *protocol.Ammo) {

	defer tool.TimeConsuming()()

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
	defer tool.TimeConsuming()()
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

func (s *Sniper) wrap() {
	defer tool.TimeConsuming()()
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

		copy(b, body)

		s.sendCache = s.sendCache[anchor:]

		id := atomic.AddUint32(&s.sendid, 1)

		ammo := protocol.Ammo{
			Id:   id - 1,
			Kind: protocol.NORMAL,
			Body: b,
		}

		s.ammoBag = append(s.ammoBag, &ammo)

		//_ = s.writerBlocker.Pass()

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
}

func (s *Sniper) deferSend(b []byte) (n int, err error) {

	defer tool.TimeConsuming()()
	n = len(b)
	s.mu.Lock()
loop:
	remain := s.sendCacheSize - int64(len(s.sendCache))

	if remain <= 0 {
		s.mu.Unlock()
		s.shoot()
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

	fmt.Println("closeeeeeeeeee")

	if s.isClose {
		return
	}

	if s.isdefer {

	loop:

		s.mu.Lock()
		if len(s.ammoBag) == 0 {
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
