package sharpshooter

import (
	"errors"
	"github.com/soyum2222/sharpshooter/protocol"
	"sync/atomic"
)

func (s *Sniper) wrapfec() {

loop:
	remain := s.winSize - int32(len(s.ammoBag))

	if remain <= 0 {
		return
	}

	l := len(s.sendBuffer)

	if l == 0 {
		return
	}

	// anchor is mark sendCache current op index
	var anchor int64

	if int64(l) < int64(s.fece.dataShards)*s.packageSize {
		anchor = int64(l)
	} else {
		anchor = int64(s.fece.dataShards) * s.packageSize
	}

	body := s.sendBuffer[:anchor]

	// body will be copied
	shard, err := s.fece.encode(body)
	if err != nil {
		s.errorContainer.Store(err)
		select {
		case <-s.errorSign:
		default:
			s.errorContainer.Store(errors.New(err.Error()))
			s.chanCloser.closeChan(s.errorSign)
		}
		return
	}

	s.sendBuffer = removeByte(s.sendBuffer, int(anchor))

	for _, v := range shard {

		ammo := protocol.Ammo{
			Id:   atomic.AddUint32(&s.sendId, 1) - 1,
			Kind: protocol.NORMAL,
			Body: v,
		}

		s.addEffectivePacket(1)
		s.ammoBag = append(s.ammoBag, &ammo)
	}

	goto loop
}

func (s *Sniper) wrapnoml() {

	remain := s.winSize - int32(len(s.ammoBag))

	for i := 0; i < int(remain); i++ {

		l := len(s.sendBuffer)

		if l == 0 {
			return
		}

		// anchor is mark sendCache current op index
		var anchor int64

		if int64(l) < s.packageSize {
			anchor = int64(l)
		} else {
			anchor = s.packageSize
		}

		id := atomic.AddUint32(&s.sendId, 1)

		ammo := protocol.Ammo{
			Id:   id - 1,
			Kind: protocol.NORMAL,
			Body: s.sendBuffer[:anchor],
		}
		s.sendBuffer = s.sendBuffer[anchor:]

		s.addEffectivePacket(1)
		s.ammoBag = append(s.ammoBag, &ammo)
	}
}
