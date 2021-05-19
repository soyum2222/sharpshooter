package sharpshooter

import (
	"fmt"
	"github.com/soyum2222/sharpshooter/protocol"
	"time"
)

func (s *Sniper) rcvnoml(ammo *protocol.Ammo) {

	s.bemu.Lock()
	defer s.bemu.Unlock()

	// eg: int8(0000 0000) - int8(1111 1111) = int8(0 - -1) = 1
	// eg: int8(1000 0001) - int8(1000 0000) = int8(-127 - -128) = 1
	// eg: int8(1000 0000) - int8(0111 1111) = int8(-128 - 127) = 1
	bagIndex := int(ammo.Id) - int(s.rcvId)

	if int(bagIndex) >= len(s.rcvAmmoBag) {
		return
	}

	if s.debug {
		fmt.Printf("%d	receive to %s seq :%d \n", time.Now().UnixNano(), s.aim.String(), ammo.Id)
	}

	s.ack(ammo.Id)

	// id < currentId , lost the ack
	if ammo.Id < s.rcvId {
		return
	}

	if s.rcvAmmoBag[bagIndex] == nil {
		s.rcvAmmoBag[bagIndex] = ammo
	}

	var anchor int

	for i := 0; i < len(s.rcvAmmoBag); i++ {

		if s.rcvAmmoBag[i] != nil {
			s.rcvCache = append(s.rcvCache, s.rcvAmmoBag[i].Body...)
			protocol.BytePool.Put(s.rcvAmmoBag[i].Body)
			s.rcvId++
			anchor++
		} else {
			break
		}
	}

	// cut off
	if anchor >= len(s.rcvAmmoBag) {
		s.rcvAmmoBag = s.rcvAmmoBag[:0]
	} else {
		length := len(s.rcvAmmoBag)
		newrcv := make([]*protocol.Ammo, length)
		s.rcvAmmoBag = s.rcvAmmoBag[anchor:]

		copy(newrcv, s.rcvAmmoBag)
		s.rcvAmmoBag = newrcv
	}

	if s.isClose {
		return
	}

	select {
	case s.readBlock <- struct{}{}:
	default:
	}
}

func (s *Sniper) rcvfec(ammo *protocol.Ammo) {

	s.bemu.Lock()
	defer s.bemu.Unlock()

	// eg: int8(0000 0000) - int8(1111 1111) = int8(0 - -1) = 1
	// eg: int8(1000 0001) - int8(1000 0000) = int8(-127 - -128) = 1
	// eg: int8(1000 0000) - int8(0111 1111) = int8(-128 - 127) = 1
	bagIndex := int(ammo.Id) - int(s.rcvId)

	// id < currentId , lost the ack
	if ammo.Id < s.rcvId {
		s.ack(ammo.Id)
		return
	}

	if bagIndex >= len(s.rcvAmmoBag) {
		return
	}

	s.ack(ammo.Id)
	if s.rcvAmmoBag[bagIndex] == nil {
		s.rcvAmmoBag[bagIndex] = ammo
	}

	var anchor int
	for i := 0; i < len(s.rcvAmmoBag); {

		blocks := make([][]byte, s.fecd.dataShards+s.fecd.parShards)
		var empty int

		for j := 0; j < s.fecd.dataShards+s.fecd.parShards && i < len(s.rcvAmmoBag); j++ {

			if s.rcvAmmoBag[i+j] == nil {
				blocks[j] = nil
				empty++
			} else {
				blocks[j] = s.rcvAmmoBag[i+j].Body
			}
		}

		if empty > s.fecd.parShards {
			break
		}

		data, err := s.fecd.decode(blocks)
		if err == nil {

			anchor += s.fecd.dataShards + s.fecd.parShards

			s.rcvCache = append(s.rcvCache, data...)

			s.rcvId += uint32(s.fecd.dataShards + s.fecd.parShards)

			for i2 := 0; i2 < s.fecd.dataShards+s.fecd.parShards; i2++ {
				if s.rcvAmmoBag[i2] != nil {
					protocol.BytePool.Put(s.rcvAmmoBag[i2].Body)
				}
			}

			i += (s.fecd.dataShards + s.fecd.parShards)

		} else {
			break
		}
	}

	// cut off
	if anchor > len(s.rcvAmmoBag) {
		s.rcvAmmoBag = s.rcvAmmoBag[:0]
	} else {
		length := len(s.rcvAmmoBag)
		newrcv := make([]*protocol.Ammo, length)
		s.rcvAmmoBag = s.rcvAmmoBag[anchor:]

		copy(newrcv, s.rcvAmmoBag)
		s.rcvAmmoBag = newrcv
	}

	if s.isClose {
		return
	}

	select {

	case s.readBlock <- struct{}{}:

	default:

	}
}
