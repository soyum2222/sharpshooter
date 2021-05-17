package sharpshooter

import (
	"github.com/soyum2222/sharpshooter/protocol"
	"net"
	"time"
)

func firstHandShack(h *headquarters, remote net.Addr) {

	_, ok := h.Snipers.Load(remote.String())
	if ok {
		return
	}

	sn := NewSniper(h.conn, remote.(*net.UDPAddr))

	h.Snipers.Store(remote.String(), sn)

	ammo := protocol.Ammo{
		Id:   0,
		Kind: protocol.SECONDHANDSHACK,
		Body: nil,
	}

	go func() {

		var try int
		ticker := time.NewTicker(time.Millisecond * 200)
		defer ticker.Stop()

	loop:

		if try > DEFAULT_INIT_HANDSHACK_TIMEOUT {
			return
		}

		sn.status = STATUS_SECONDHANDSHACK
		_, _ = sn.conn.WriteToUDP(protocol.Marshal(ammo), sn.aim)

		select {
		case <-ticker.C:
			try++
			goto loop

		case <-sn.handShakeSign:
			return
		}
	}()
}

func secondHandShack(h *headquarters, remote net.Addr) {
	i, ok := h.Snipers.Load(remote.String())
	if !ok {
		return
	}

	sn := i.(*Sniper)

	// verification status
	//if sn.status != STATUS_FIRSTHANDSHACK {
	//	return
	//}

	sn.status = STATUS_THIRDHANDSHACK

	ammo := protocol.Ammo{
		Id:   0,
		Kind: protocol.THIRDHANDSHACK,
		Body: nil,
	}

	_, _ = sn.conn.WriteToUDP(protocol.Marshal(ammo), sn.aim)
}

func thirdHandShack(h *headquarters, remote net.Addr) {
	i, ok := h.Snipers.Load(remote.String())
	if !ok {
		return
	}

	sn := i.(*Sniper)

	//if sn.status != STATUS_SECONDHANDSHACK {
	//	return
	//}

	sn.status = STATUS_NORMAL

	SystemTimedSched.Put(sn.healthMonitor, time.Now().Add(time.Second*3))

	select {
	case sn.handShakeSign <- struct{}{}:
	default:
	}

	h.accept <- sn
}
