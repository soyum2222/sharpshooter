package sharpshooter

import (
	"context"
	"encoding/binary"
	"errors"
	"github.com/soyum2222/sharpshooter/protocol"
	"net"
	"sync/atomic"
	"time"
)

type headquarters struct {
	conn           *net.UDPConn
	Snipers        map[string]*Sniper
	blockSign      chan struct{}
	accept         chan *Sniper
	errorSign      chan struct{}
	errorContainer atomic.Value
	closeSign      chan struct{}
}

func Dial(addr string) (*Sniper, error) {

	udpaddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, err
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	sn := NewSniper(conn, udpaddr)
	sn.noLeader = true

	c := make(chan error, 1)

	go func() {

		secondhand := make([]byte, 10)

		ctx, _ := context.WithTimeout(context.Background(), time.Second*6)

		select {

		case <-func() chan struct{} {
			ch := make(chan struct{})
			_, err = sn.conn.Read(secondhand)
			if err != nil {
				c <- err
			}
			close(ch)
			return ch
		}():

		case <-ctx.Done():
			_ = conn.Close()

		}

		if protocol.Unmarshal(secondhand).Kind != protocol.SECONDHANDSHACK {
			c <- errors.New("handshake package error")
		}

		c <- nil

	}()

	var i int
loop:

	if i > 6 {
		return nil, errors.New("dial timeout")
	}

	ammo := protocol.Ammo{
		Kind: protocol.FIRSTHANDSHACK,
	}

	_, err = sn.conn.WriteToUDP(protocol.Marshal(ammo), sn.aim)
	if err != nil {
		return nil, err
	}

	select {

	case <-ticker.C:
		i++
		goto loop
	case err = <-c:
		if err != nil {
			return nil, err
		}
	}

	ammo.Kind = protocol.THIRDHANDSHACK

	_, err = sn.conn.WriteToUDP(protocol.Marshal(ammo), sn.aim)
	if err != nil {
		return nil, err
	}

	go sn.ackTimer()

	sn.timeoutTimer = time.NewTimer(time.Duration(sn.rto) * time.Nanosecond)
	go sn.shooter()
	sn.healthTimer = time.NewTimer(time.Second * 3)
	go sn.healthMonitor()
	go sn.monitor()

	return sn, nil

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
		blockSign: make(chan struct{}, 0),
		accept:    make(chan *Sniper, 10),
		errorSign: make(chan struct{}, 0),
		closeSign: make(chan struct{}, 0),
	}
}

func (h *headquarters) Accept() (*Sniper, error) {
	sn := <-h.accept
	go sn.ackTimer()
	sn.timeoutTimer = time.NewTimer(time.Duration(sn.rto) * time.Nanosecond)
	go sn.shooter()
	return sn, nil
}

func (h *headquarters) Close() {

}

func (h *headquarters) WriteToAddr(b []byte, addr *net.UDPAddr) (int, error) {

	sn, ok := h.Snipers[addr.String()]
	if !ok {
		return 0, errors.New("the addr is not dial")
	}

	return sn.Write(b)

}

func (h *headquarters) clear() {

	for {
		time.Sleep(time.Second * 5)

		select {
		case <-h.closeSign:
			return
		default:

			for k, v := range h.Snipers {
				if v.isClose {
					delete(h.Snipers, k)
				}
			}
		}

	}
}

// TODO
// use dial a sniper will leak!
func (h *headquarters) monitor() {
	go h.clear()

	b := make([]byte, DEFAULT_INIT_PACKSIZE+20)
	for {

		select {
		case <-h.closeSign:

		default:

			n, remote, err := h.conn.ReadFrom(b)
			if err != nil {
				h.errorContainer.Store(err)
				close(h.errorSign)
			}

			msg := protocol.Unmarshal(b[:n])
			switch msg.Kind {

			case protocol.FIRSTHANDSHACK:
				firstHandShack(h, remote)

			case protocol.SECONDHANDSHACK:
				secondHandShack(h, remote)

			case protocol.THIRDHANDSHACK:
				thirdHandShack(h, remote)

			default:
				sn, ok := h.Snipers[remote.String()]
				if !ok {
					continue
				}

				routing(sn, msg)

				select {
				case h.blockSign <- struct{}{}:
				default:
				}

			}

		}

	}
}

func routing(sn *Sniper, msg protocol.Ammo) {

	switch msg.Kind {

	case protocol.ACK:

		l := len(msg.Body)
		if l%4 != 0 {
			return
		}

		ids := make([]uint32, 0, l/4)
		for i := 0; i < l; i += 4 {
			ids = append(ids, binary.BigEndian.Uint32(msg.Body[i:i+4]))
		}

		sn.handleAck(ids)

	case protocol.CLOSE:

		if msg.Id == sn.rcvId {

			sn.rcvId++

			sn.ack(msg.Id)

			go func() {
				var try int
			l:
				sn.bemu.Lock()

				// TODO
				// here have possible a close message in the ammoBge
				// now I don't know how to do
				// temporarily add try count
				if len(sn.ammoBag) == 0 || try > 10 {
					sn.writerBlocker.Close()

					if sn.isClose {
						return
					}

					sn.isClose = true

					sn.ackSign.Close()
					close(sn.closeChan)
					if sn.noLeader {
						_ = sn.conn.Close()
					}
					sn.bemu.Unlock()
					_, err := sn.conn.WriteToUDP(protocol.Marshal(protocol.Ammo{
						Kind: protocol.CLOSERESP,
						Body: nil,
					}), sn.aim)
					if err != nil {
						return
					}

				} else {
					sn.bemu.Unlock()
					time.Sleep(time.Second)
					try++
					goto l

				}

			}()

		}

	case protocol.CLOSERESP:
		close(sn.closeChan)

	case protocol.HEALTHCHECK:
		_, _ = sn.conn.WriteToUDP(protocol.Marshal(protocol.Ammo{
			Kind: protocol.HEALTCHRESP,
		}), sn.aim)

	case protocol.HEALTCHRESP:

		atomic.StoreInt32(&sn.healthTryCount, 0)

		t := time.Now().UnixNano()

		sn.calrto(t - sn.timeFlag)

	case protocol.NORMAL:
		sn.beShot(&msg)

	}

}

func (h *headquarters) ReadFrom(b []byte) (int, net.Addr, error) {

	for {

		for _, v := range h.Snipers {

			if len(v.rcvAmmoBag) == 0 || v.rcvAmmoBag[0] == nil {
				continue
			}

			n, err := v.Read(b)
			return n, v.aim, err

		}

		select {
		case <-h.blockSign:
		case <-h.errorSign:
			return 0, nil, h.errorContainer.Load().(error)
		}

	}

}
