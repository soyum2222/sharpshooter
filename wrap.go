package sharpshooter

import (
	"errors"
	"github.com/soyum2222/sharpshooter/protocol"
	"sync/atomic"
)

func (s *Sniper) wrapfec() {

loop:
	remain := s.maxWin - int32(len(s.ammoBag))

	if remain <= 0 {
		return
	}

	l := len(s.sendCache)

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

	body := s.sendCache[:anchor]

	// body will be copied
	shard, err := s.fece.encode(body)
	if err != nil {
		s.errorContainer.Store(err)
		select {
		case <-s.errorSign:
		default:
			s.errorContainer.Store(errors.New(err.Error()))
			closeChan(s.errorSign)
		}
		return
	}

	s.sendCache = s.sendCache[anchor:]

	for _, v := range shard {

		ammo := protocol.Ammo{
			Id:   atomic.AddUint32(&s.sendId, 1) - 1,
			Kind: protocol.NORMAL,
			Body: v,
		}

		s.ammoBag = append(s.ammoBag, &ammo)

	}

	goto loop
}

func (s *Sniper) wrapnoml() {

	remain := s.maxWin - int32(len(s.ammoBag))

	for i := 0; i < int(remain); i++ {

		l := len(s.sendCache)

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

		body := s.sendCache[:anchor]

		s.sendCache = s.sendCache[anchor:]

		id := atomic.AddUint32(&s.sendId, 1)

		ammo := protocol.Ammo{
			Id:   id - 1,
			Kind: protocol.NORMAL,
			Body: body,
		}

		s.ammoBag = append(s.ammoBag, &ammo)

	}

}
