package core

import (
	"fmt"
	"net"
	"runtime"
	"sharpshooter/protocol"
	"sync"
	"sync/atomic"
	"time"
)

const (
	shooting = 1 << iota
	waittimeout
)

type Sniper struct {
	aim                  *net.UDPAddr
	conn                 *net.UDPConn
	ammoBag              []*protocol.Ammo
	beShotAmmoBag        []*protocol.Ammo
	maxAmmoBag           int64
	sendid               uint32
	beShotCurrentId      uint32
	maxWindows           int64
	currentWindowStartId uint32
	currentWindowEndId   uint32
	timeout              int64
	shootTime            int64
	shootStatus          int32
	handshakesign        chan struct{}
	readblock            chan struct{}
	timeoutticker        *time.Ticker
	mu                   sync.RWMutex
	bemu                 sync.Mutex
}

func NewSniper(conn *net.UDPConn, aim *net.UDPAddr, timeout int64) *Sniper {

	return &Sniper{
		aim:           aim,
		conn:          conn,
		timeout:       timeout,
		beShotAmmoBag: make([]*protocol.Ammo, 10),
		handshakesign: make(chan struct{}, 1),
		readblock:     make(chan struct{}, 0),
		maxAmmoBag:    100,
		maxWindows:    100,
		mu:            sync.RWMutex{},
		bemu:          sync.Mutex{},
	}

}

func (s *Sniper) load(ammo protocol.Ammo) {

loop:
	s.mu.Lock()
	if len(s.ammoBag) > int(s.maxAmmoBag) {
		s.mu.Unlock()
		runtime.Gosched()
		goto loop
	}

	if atomic.LoadInt32(&s.shootStatus)&(shooting|waittimeout) == 0 {
		s.ammoBag = append(s.ammoBag, &ammo)
		go s.Shot()
	} else {
		s.ammoBag = append(s.ammoBag, &ammo)
	}
	s.mu.Unlock()

}

func (s *Sniper) Shot() {

	if s.timeout == 0 {
		s.timeout = int64(time.Second)
	}
	s.timeoutticker = time.NewTicker(time.Duration(s.timeout) * time.Nanosecond)

loop:

	status := atomic.LoadInt32(&s.shootStatus)
	if status&shooting >= 1 {
		return
	}

	if !atomic.CompareAndSwapInt32(&s.shootStatus, status, status|shooting) {
		return
	}

	s.mu.Lock()
	if s.ammoBag == nil || len(s.ammoBag) == 0 {
		atomic.StoreInt32(&s.shootStatus, 0)
		s.timeoutticker.Stop()
		s.mu.Unlock()
		return
	}

	for k, _ := range s.ammoBag {

		if s.ammoBag[k] == nil {
			continue
		}

		if int64(k) > s.maxWindows {
			break
		}

		s.currentWindowEndId = s.currentWindowStartId + uint32(k)

		_, err := s.conn.WriteToUDP(protocol.Marshal(*s.ammoBag[k]), s.aim)
		if err != nil {
			panic(err)
		}
	}

	s.mu.Unlock()

	for {

		status = atomic.LoadInt32(&s.shootStatus)
		if atomic.CompareAndSwapInt32(&s.shootStatus, status, (status&(^shooting))|waittimeout) {
			break
		}

	}

	select {
	case <-s.timeoutticker.C:

		for {
			status = atomic.LoadInt32(&s.shootStatus)
			if atomic.CompareAndSwapInt32(&s.shootStatus, status, (status & (^waittimeout))) {
				break
			}
		}
		goto loop
	}

}

func (s *Sniper) ack(id uint32) {

	ammo := protocol.Ammo{
		Id:   id,
		Kind: protocol.ACK,
		Body: nil,
	}

	_, err := s.conn.WriteToUDP(protocol.Marshal(ammo), s.aim)

	if err != nil {
		panic(err)
	}
}

func (s *Sniper) score(id uint32) {

	fmt.Println("id", id)
	if id < atomic.LoadUint32(&s.currentWindowStartId) {
		return
	}

	index := int(id) - int(atomic.LoadUint32(&s.currentWindowStartId))

	if int(index) > len(s.ammoBag) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.ammoBag[index] = nil

	if id == atomic.LoadUint32(&s.currentWindowStartId) {

		for i := 0; i < len(s.ammoBag); i++ {

			// check is score  if index 0 is score then offset it ,on the not score index
			if s.ammoBag[i] != nil {
				//fmt.Println("current index ", index)
				//
				//fmt.Println("current id ", id)
				//fmt.Println("currentwindowstart:", atomic.LoadUint32(&s.currentWindowStartId))
				//fmt.Println(s.ammoBag)
				s.ammoBag = s.ammoBag[i:]
				//fmt.Println(s.ammoBag)
				atomic.AddUint32(&s.currentWindowStartId, uint32(i))
				//s.currentWindowStartId = s.currentWindowStartId + uint32(i) + 1
				//fmt.Println("currentwindowstart  ed:", atomic.LoadUint32(&s.currentWindowStartId))
				break
			}

		}

		if id == s.currentWindowEndId {
			go s.Shot()
		}

	}
}

func (s *Sniper) BeShot(ammo *protocol.Ammo) {

	s.bemu.Lock()
	defer s.bemu.Unlock()

	bagIndex := ammo.Id - s.beShotCurrentId

	if ammo.Id < s.beShotCurrentId {
		fmt.Println("ammo.Id < s.beShotCurrentId", ammo.Id)
		s.ack(ammo.Id)
		return
	}

	if int(bagIndex) >= len(s.beShotAmmoBag) {
		fmt.Println("int(bagIndex) >= len(s.beShotAmmoBag)", ammo.Id)
		return
	}

	if s.beShotAmmoBag[bagIndex] == nil {
		s.beShotAmmoBag[bagIndex] = ammo
	}

	select {

	case s.readblock <- struct{}{}:

	default:

	}

	s.ack(ammo.Id)
}

func PrintAmmo(s []*protocol.Ammo) {

	for _, ammo := range s {

		fmt.Print(string(ammo.Body))
		fmt.Print("  ")

	}
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
				PrintAmmo(s.beShotAmmoBag)
				s.beShotAmmoBag = append(s.beShotAmmoBag[1:], nil)
				fmt.Println("ed")
				PrintAmmo(s.beShotAmmoBag)
				s.beShotCurrentId++
			}
			break

		} else {

			PrintAmmo(s.beShotAmmoBag)

			s.beShotAmmoBag = append(s.beShotAmmoBag[1:], nil)

			fmt.Println("ed")
			PrintAmmo(s.beShotAmmoBag)

			s.beShotCurrentId++
			continue
		}
	}

	s.bemu.Unlock()

	if n <= 0 {
		<-s.readblock
		goto loop
	}

	fmt.Println(b)
	fmt.Println(string(b))
	return
}

func (s *Sniper) Write(b []byte) (n int, err error) {

	id := atomic.AddUint32(&s.sendid, 1)

	ammo := protocol.Ammo{
		Length: 0,
		Id:     id - 1,
		Kind:   5,
		Body:   b,
	}

	s.load(ammo)
	return len(b), nil
}
