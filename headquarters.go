package sharpshooter

import (
	"errors"
	"net"
	"runtime"
	"sharpshooter/protocol"
	"sync/atomic"
	"time"
)

type headquarters struct {
	conn      *net.UDPConn
	Snipers   map[string]*Sniper
	blocksign chan struct{}
	accept    chan *Sniper
}

func Dial(addr string) (*Sniper, error) {

	h := NewHeadquarters()

	udpaddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	// todo timeout
	err = h.Dial(udpaddr)
	if err != nil {
		return nil, err
	}

	go h.monitor()

	return h.Snipers[udpaddr.String()], nil
}

func Listen(addr *net.UDPAddr) (*headquarters, error) {

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}

	h := NewHeadquarters()
	h.conn = conn

	go h.monitor()

	return h, nil

}

func NewHeadquarters() *headquarters {

	return &headquarters{
		Snipers:   map[string]*Sniper{},
		blocksign: make(chan struct{}, 0),
		accept:    make(chan *Sniper, 10),
	}
}

func (h *headquarters) Accept() (*Sniper, error) {
	sn := <-h.accept
	go sn.ackSender()
	go sn.shooter()
	return sn, nil
}

func (h *headquarters) WriteToAddr(b []byte, addr *net.UDPAddr) error {

	sn, ok := h.Snipers[addr.String()]
	if !ok {
		return errors.New("the addr is not dial")
	}
	sn.ammoBagCach <- protocol.Unmarshal(b)

	return nil
}

func (h *headquarters) Dial(addr *net.UDPAddr) error {

	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return err
	}

	h.conn = conn

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	sn := NewSniper(conn, addr)

	//sn.rtt = time.Now().UnixNano()

	// receive second handshake
	c := make(chan error, 1)
	go func() {

		secondhand := make([]byte, 10)

		_, err = sn.conn.Read(secondhand)
		if err != nil {
			c <- err
		}

		if protocol.Unmarshal(secondhand).Kind != protocol.SECONDHANDSHACK {
			c <- errors.New("handshake package error")
		}

		c <- nil

	}()

loop:
	ammo := protocol.Ammo{
		Kind: protocol.FIRSTHANDSHACK,
	}

	_, err = sn.conn.WriteToUDP(protocol.Marshal(ammo), sn.aim)
	if err != nil {
		return err
	}

	select {

	case <-ticker.C:
		goto loop
	case err = <-c:
		if err != nil {
			return err
		}
	}

	//sn.rtt = (time.Now().UnixNano() - sn.rtt)
	////
	//if sn.rtt < DEFAULT_INIT_MIN_TIMEOUT {
	//	sn.rtt = DEFAULT_INIT_MIN_TIMEOUT
	//}

	ammo.Kind = protocol.THIRDHANDSHACK

	_, err = sn.conn.WriteToUDP(protocol.Marshal(ammo), sn.aim)
	if err != nil {
		return err
	}

	h.Snipers[addr.String()] = sn

	go sn.ackSender()
	go sn.shooter()
	sn.healthTimer = time.NewTimer(time.Second * 3)
	go sn.healthMonitor()

	return nil
}

func (h *headquarters) clear() {

	for {
		time.Sleep(time.Second * 5)

		for k, v := range h.Snipers {
			if v.isClose {
				delete(h.Snipers, k)
			}
		}

	}
}
func (h *headquarters) monitor() {
	go h.clear()

	b := make([]byte, DEFAULT_INIT_PACKSIZE+20)
	for {

		n, remote, err := h.conn.ReadFrom(b)
		if err != nil {
			return
		}

		//count++

		msg := protocol.Unmarshal(b[:n])

		h.routing(msg, remote)
	}

}

func (h *headquarters) routing(msg protocol.Ammo, remote net.Addr) {

	switch msg.Kind {

	case protocol.ACK:

		sn, ok := h.Snipers[remote.String()]
		if !ok {
			return
		}
		//sn.healthTimer.Reset(time.Second * DEFAULT_INIT_HEALTHTICKER)
		sn.score(msg.Id)

	case protocol.FIRSTHANDSHACK:

		sn := NewSniper(h.conn, remote.(*net.UDPAddr))

		h.Snipers[remote.String()] = sn

		ammo := protocol.Ammo{
			Id:   0,
			Kind: protocol.SECONDHANDSHACK,
			Body: nil,
		}

		go func() {

			var tryconut int
			ticker := time.NewTicker(time.Second)

		loop:
			if tryconut > 6 {
				return
			}

			sn.conn.WriteToUDP(protocol.Marshal(ammo), sn.aim)

			select {

			case <-ticker.C:
				tryconut++
				goto loop
			case <-sn.handshakesign:
				return
			}

		}()

	case protocol.SECONDHANDSHACK:

		sn, ok := h.Snipers[remote.String()]
		if !ok {
			return
		}

		ammo := protocol.Ammo{
			Id:   0,
			Kind: protocol.THIRDHANDSHACK,
			Body: nil,
		}

		sn.conn.WriteToUDP(protocol.Marshal(ammo), sn.aim)

	case protocol.THIRDHANDSHACK:

		sn, ok := h.Snipers[remote.String()]
		if !ok {
			return
		}

		//sn.rtt = time.Now().UnixNano() - sn.rtt
		//if sn.rtt < DEFAULT_INIT_MIN_TIMEOUT {
		//	sn.rtt = DEFAULT_INIT_MIN_TIMEOUT
		//}

		sn.healthTimer = time.NewTimer(time.Second * 3)
		go sn.healthMonitor()

		select {
		case sn.handshakesign <- struct{}{}:
		default:

		}

		h.accept <- sn

	case protocol.CLOSE:

		sn, ok := h.Snipers[remote.String()]
		if !ok {
			return
		}

		if msg.Id == sn.beShotCurrentId {

			sn.beShotCurrentId++
			//sn.isClose = true

			sn.ack(msg.Id)

			go func() {
			l:

				sn.bemu.Lock()
				if len(sn.ammoBag) == 0 {

					sn.writerBlocker.Close()

					sn.isClose = true

					sn.acksign.Close()
					close(sn.closeChan)
					sn.bemu.Unlock()
					delete(h.Snipers, remote.String())
					_, err := sn.conn.WriteToUDP(protocol.Marshal(protocol.Ammo{
						Kind: protocol.CLOSERESP,
						Body: nil,
					}), sn.aim)
					if err != nil {
						panic(err)
					}

				} else {

					sn.bemu.Unlock()

					runtime.Gosched()
					goto l

				}

			}()

		}

	case protocol.CLOSERESP:
		sn, ok := h.Snipers[remote.String()]
		if !ok {
			return
		}
		close(sn.closeChan)

	case protocol.HEALTHCHECK:
		sn, ok := h.Snipers[remote.String()]
		if !ok {
			return
		}
		_, _ = sn.conn.WriteToUDP(protocol.Marshal(protocol.Ammo{
			Kind: protocol.HEALTCHRESP,
		}), sn.aim)

	case protocol.HEALTCHRESP:
		sn, ok := h.Snipers[remote.String()]
		if !ok {
			return
		}

		atomic.StoreInt32(&sn.healthTryCount, 0)

		t := time.Now().UnixNano()

		sn.calrto(t - sn.timeFlag)

		// new rtt 15% come form new value , 85% come form old value
		//atomic.StoreInt64(&sn.rtt, int64(0.125*float64(t-sn.timeFlag)+(1-0.125)*float64(atomic.LoadInt64(&sn.rtt))))

	default:
		sn, ok := h.Snipers[remote.String()]
		if !ok {
			return
		}

		//sn.healthTimer.Reset(time.Second * DEFAULT_INIT_HEALTHTICKER)

		sn.beShot(&msg)

		select {

		case h.blocksign <- struct{}{}:

		default:

		}

	}

}

func (h *headquarters) ReadFrom(b []byte) (int, net.Addr, error) {

	for {

		for _, v := range h.Snipers {

			if len(v.beShotAmmoBag) == 0 || v.beShotAmmoBag[0] == nil {
				continue
			}

			n, err := v.Read(b)
			return n, v.aim, err

		}

		<-h.blocksign
	}
}
