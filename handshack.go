package sharpshooter

import (
	"github.com/soyum2222/sharpshooter/protocol"
	"math/rand"
	"net"
	"time"
)

func init() {
	rand.Seed(time.Now().Unix())
}
func firstHandShack(h *headquarters, remote net.Addr) {

	_, ok := h.Snipers.Load(remote.String())
	if ok {
		return
	}

	sn := NewSniper(h.conn, remote.(*net.UDPAddr))

	// verification status
	// avoid malicious packages
	if sn.status != STATUS_NONE {
		return
	}

	h.Snipers.Store(remote.String(), sn)

	sn.latestSendId = rand.Uint32()
	ammo := protocol.Ammo{
		Id:   sn.latestSendId,
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

func secondHandShack(h *headquarters, remote net.Addr, id uint32) {
	i, ok := h.Snipers.Load(remote.String())
	if !ok {
		return
	}

	sn := i.(*Sniper)

	// verification status
	// avoid malicious packages
	if sn.status != STATUS_NONE {
		return
	}

	sn.status = STATUS_THIRDHANDSHACK

	ammo := protocol.Ammo{
		Id:   id,
		Kind: protocol.THIRDHANDSHACK,
		Body: nil,
	}

	_, _ = sn.conn.WriteToUDP(protocol.Marshal(ammo), sn.aim)
}

func thirdHandShack(h *headquarters, remote net.Addr, id uint32) {
	i, ok := h.Snipers.Load(remote.String())
	if !ok {
		return
	}

	sn := i.(*Sniper)

	// verification status
	// avoid malicious packages
	if sn.status != STATUS_SECONDHANDSHACK {
		return
	}

	if id != sn.latestSendId {
		h.Snipers.Delete(remote.String())
		return
	}

	sn.latestSendId = 0
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
