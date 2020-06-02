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

func Dial(addr *net.UDPAddr) (*Sniper, error) {

	h := NewHeadquarters()

	// todo timeout
	err := h.Dial(addr)
	if err != nil {
		return nil, err
	}

	go h.monitor()

	return h.Snipers[addr.String()], nil
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

	sn := NewSniper(conn, addr, time.Now().UnixNano())

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

	sn.timeout = (time.Now().UnixNano() - sn.timeout)

	if sn.timeout < DEFAULT_INIT_MIN_TIMEOUT {
		sn.timeout = DEFAULT_INIT_MIN_TIMEOUT
	}

	ammo.Kind = protocol.THIRDHANDSHACK

	_, err = sn.conn.WriteToUDP(protocol.Marshal(ammo), sn.aim)
	if err != nil {
		return err
	}

	h.Snipers[addr.String()] = sn

	go sn.ackSender()
	sn.healthTimer = time.NewTimer(time.Second * 3)
	go sn.healthMonitor()

	return nil
}

func (h *headquarters) monitor() {
	//
	//defer func() {
	//	if e := recover(); e != nil {
	//		fmt.Println(e)
	//	}
	//
	//}()

	b := make([]byte, 1024000)
	for {

		n, remote, err := h.conn.ReadFrom(b)
		if err != nil {
			return
		}

		msg := protocol.Unmarshal(b[:n])

		switch msg.Kind {

		case protocol.ACK:

			sn, ok := h.Snipers[remote.String()]
			if !ok {
				continue
			}
			sn.healthTimer.Reset(time.Second * DEFAULT_INIT_HEALTHTICKER)
			sn.score(msg.Id)

		case protocol.FIRSTHANDSHACK:

			sn := NewSniper(h.conn, remote.(*net.UDPAddr), time.Now().UnixNano())

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
				continue
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
				continue
			}

			sn.timeout = time.Now().UnixNano() - sn.timeout

			if sn.timeout < DEFAULT_INIT_MIN_TIMEOUT {
				sn.timeout = DEFAULT_INIT_MIN_TIMEOUT
			}

			sn.healthTimer = time.NewTimer(time.Second * 2)
			go sn.healthMonitor()

			sn.handshakesign <- struct{}{}

			h.accept <- sn

		case protocol.CLOSE:

			sn, ok := h.Snipers[remote.String()]
			if !ok {
				continue
			}

			if msg.Id == sn.beShotCurrentId {

				delete(h.Snipers, remote.String())

				close(sn.ammoBagCach)

				sn.ack(msg.Id)

				go func() {
				l:
					if len(sn.ammoBagCach) == 0 {

						sn.isClose = true
						close(sn.closeChan)
						//close(sn.acksign)

						_, err := sn.conn.WriteToUDP(protocol.Marshal(protocol.Ammo{
							Kind: protocol.CLOSERESP,
							Body: nil,
						}), sn.aim)
						if err != nil {
							panic(err)
						}

					} else {

						runtime.Gosched()
						goto l

					}

				}()

			}

		case protocol.CLOSERESP:
			sn, ok := h.Snipers[remote.String()]
			if !ok {
				continue
			}
			close(sn.closeChan)

		case protocol.HEALTHCHECK:
			sn, ok := h.Snipers[remote.String()]
			if !ok {
				continue
			}
			sn.conn.WriteToUDP(protocol.Marshal(protocol.Ammo{
				Kind: protocol.HEALTCHRESP,
			}), sn.aim)

		case protocol.HEALTCHRESP:
			sn, ok := h.Snipers[remote.String()]
			if !ok {
				continue
			}

			atomic.StoreInt32(&sn.healthTryCount, 0)
			t := time.Now().UnixNano()
			sn.timeout = t - sn.timeFlag
			if sn.timeout < DEFAULT_INIT_MIN_TIMEOUT {
				sn.timeout = DEFAULT_INIT_MIN_TIMEOUT
			}

		default:
			sn, ok := h.Snipers[remote.String()]
			if !ok {
				continue
			}

			sn.healthTimer.Reset(time.Second * DEFAULT_INIT_HEALTHTICKER)

			sn.BeShot(&msg)

			select {

			case h.blocksign <- struct{}{}:

			default:

			}

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
