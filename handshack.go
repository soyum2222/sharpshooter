package sharpshooter

import (
	"github.com/soyum2222/sharpshooter/protocol"
	"net"
	"time"
)

func firstHandShack(h *headquarters, remote net.Addr) {

	sn := NewSniper(h.conn, remote.(*net.UDPAddr))

	h.Snipers[remote.String()] = sn

	ammo := protocol.Ammo{
		Id:   0,
		Kind: protocol.SECONDHANDSHACK,
		Body: nil,
	}

	go func() {

		var try int
		ticker := time.NewTicker(time.Second)

	loop:

		if try > DEFAULT_INIT_HANDSHACK_TIMEOUT {
			return
		}

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
	sn, ok := h.Snipers[remote.String()]
	if !ok {
		return
	}

	ammo := protocol.Ammo{
		Id:   0,
		Kind: protocol.THIRDHANDSHACK,
		Body: nil,
	}

	_, _ = sn.conn.WriteToUDP(protocol.Marshal(ammo), sn.aim)
}

func thirdHandShack(h *headquarters, remote net.Addr) {
	sn, ok := h.Snipers[remote.String()]
	if !ok {
		return
	}

	sn.healthTimer = time.NewTimer(time.Second * 3)
	go sn.healthMonitor()

	select {
	case sn.handShakeSign <- struct{}{}:
	default:

	}

	h.accept <- sn
}
