package sharpshooter

import (
	"context"
	"encoding/binary"
	"errors"
	"github.com/soyum2222/sharpshooter/protocol"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type headquarters struct {
	conn           *net.UDPConn
	Snipers        sync.Map
	blockSign      chan struct{}
	accept         chan *Sniper
	errorSign      chan struct{}
	errorContainer atomic.Value
	closeSign      chan struct{}
	chanCloser     chanCloser
}

func (h *headquarters) Addr() net.Addr {
	return h.conn.LocalAddr()
}

func Dial(addr string) (net.Conn, error) {

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

			// here close is absolutely safe
			close(ch)

			return ch
		}():

		case <-ctx.Done():
			_ = conn.Close()
		}

		ammo, err := protocol.Unmarshal(secondhand)
		if err != nil {
			c <- err
			return
		}

		if ammo.Kind != protocol.SECONDHANDSHACK {
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

func Listen(addr string) (*headquarters, error) {

	address, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", address)
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
		blockSign: make(chan struct{}, 0),
		accept:    make(chan *Sniper, 10),
		errorSign: make(chan struct{}, 0),
		closeSign: make(chan struct{}, 0),
	}
}

func (h *headquarters) Accept() (net.Conn, error) {
	select {
	case sn := <-h.accept:
		go sn.ackTimer()
		sn.timeoutTimer = time.NewTimer(time.Duration(sn.rto) * time.Nanosecond)
		go sn.shooter()
		return sn, nil

	case <-h.errorSign:
		return nil, h.errorContainer.Load().(error)
	}
}

func (h *headquarters) Close() error {
	h.chanCloser.closeChan(h.closeSign)
	return h.conn.Close()
}

func (h *headquarters) WriteToAddr(b []byte, addr *net.UDPAddr) (int, error) {

	sn, ok := h.Snipers.Load(addr.String())
	if !ok {
		return 0, errors.New("the addr is not dial")
	}

	return sn.(*Sniper).Write(b)
}

func (h *headquarters) clear() {

	for {
		time.Sleep(time.Second * 5)

		select {
		case <-h.closeSign:
			return
		default:

			h.Snipers.Range(func(key, value interface{}) bool {
				if value.(*Sniper).isClose {
					h.Snipers.Delete(key)
				}
				return true
			})
		}
	}
}

func (h *headquarters) monitor() {
	go h.clear()

	b := make([]byte, DEFAULT_INIT_PACKSIZE+20)
	for {

		select {
		case <-h.closeSign:
			return

		default:

			n, remote, err := h.conn.ReadFrom(b)
			if err != nil {
				h.errorContainer.Store(err)
				h.chanCloser.closeChan(h.errorSign)
			}

			if n == 0 {
				continue
			}

			msg, err := protocol.Unmarshal(b[:n])
			if err != nil {
				// bad message
				continue
			}

			switch msg.Kind {

			case protocol.FIRSTHANDSHACK:
				firstHandShack(h, remote)

			case protocol.SECONDHANDSHACK:
				secondHandShack(h, remote)

			case protocol.THIRDHANDSHACK:
				thirdHandShack(h, remote)

			default:
				sn, ok := h.Snipers.Load(remote.String())
				if !ok {
					continue
				}

				routing(sn.(*Sniper), msg)

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
		// revive close signal after func Write can't write anything
		if msg.Id == sn.rcvId {

			sn.rcvId++

			sn.ack(msg.Id)

			sn.mu.Lock()
			// find tail ammo
			tail := len(sn.ammoBag) - 1
			if tail > 0 {
				sn.ammoBag[tail].Kind = protocol.NORMALTAIL
			}
			sn.mu.Unlock()

			go sn.closeOnce.Do(func() {

				var try int
			l:
				sn.bemu.Lock()

				if len(sn.ammoBag) == 0 || try > 0xff {

					sn.writerBlocker.Close()

					if sn.isClose {
						sn.bemu.Unlock()
						return
					}

					sn.isClose = true

					sn.chanCloser.closeChan(sn.closeChan)

					if sn.noLeader {
						defer func() { _ = sn.conn.Close() }()
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
					sn.shoot()
					try++
					goto l
				}
			})
		}

	case protocol.CLOSERESP:
		sn.chanCloser.closeChan(sn.closeChan)

	case protocol.HEALTHCHECK:
		_, _ = sn.conn.WriteToUDP(protocol.Marshal(protocol.Ammo{
			Kind: protocol.HEALTCHRESP,
		}), sn.aim)

	case protocol.HEALTCHRESP:

		atomic.StoreInt32(&sn.healthTryCount, 0)

		t := time.Now().UnixNano()

		sn.calrto(t - sn.timeFlag)

	case protocol.NORMAL:
		sn.rcv(&msg)

	case protocol.NORMALTAIL:
		sn.rcv(&msg)
		sn.chanCloser.closeChan(sn.closeChan)
	}
}

func (h *headquarters) ReadFrom(b []byte) (int, net.Addr, error) {

	for {

		var ret bool
		var err error
		var n int
		var addr net.Addr

		h.Snipers.Range(func(key, value interface{}) bool {
			v := value.(*Sniper)
			if len(v.rcvAmmoBag) == 0 || v.rcvAmmoBag[0] == nil {
				return true
			}

			ret = true
			n, err = v.Read(b)
			addr = v.aim
			return false
		})

		if ret {
			return n, addr, err
		}

		select {
		case <-h.blockSign:
		case <-h.errorSign:
			return 0, nil, h.errorContainer.Load().(error)
		}
	}
}
