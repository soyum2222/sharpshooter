package protocol

import (
	"encoding/binary"
	"fmt"
	"sync/atomic"
)

const (
	ACK = iota
	NORMAL
	FIRSTHANDSHACK
	SECONDHANDSHACK
	THIRDHANDSHACK
	CLOSE
	CLOSERESP
	HEALTHCHECK
	HEALTCHRESP
	NORMALTAIL
	OUTOFAMMO
)

//var BytePool sync.Pool
//
//func init() {
//	BytePool.New = func() interface{} {
//		return make([]byte, 1800)
//	}
//}

type Ammo struct {
	Length     uint32
	Id         uint32
	Kind       uint16
	proof      uint32
	Body       []byte
	ackCount   uint32
	shootCount uint32
}

var table = []uint8{0, 1, 1, 2, 1, 2, 2, 3, 1, 2, 2, 3, 2, 3, 3, 4, 1, 2, 2, 3, 2, 3, 3, 4, 2, 3, 3, 4, 3, 4, 4, 5, 1,
	2, 2, 3, 2, 3, 3, 4, 2, 3, 3, 4, 3, 4, 4, 5, 2, 3, 3, 4, 3, 4, 4, 5, 3, 4, 4, 5, 4, 5, 5, 6, 1, 2, 2, 3, 2, 3, 3, 4,
	2, 3, 3, 4, 3, 4, 4, 5, 2, 3, 3, 4, 3, 4, 4, 5, 3, 4, 4, 5, 4, 5, 5, 6, 2, 3, 3, 4, 3, 4, 4, 5, 3, 4, 4, 5, 4, 5, 5,
	6, 3, 4, 4, 5, 4, 5, 5, 6, 4, 5, 5, 6, 5, 6, 6, 7, 1, 2, 2, 3, 2, 3, 3, 4, 2, 3, 3, 4, 3, 4, 4, 5, 2, 3, 3, 4, 3, 4,
	4, 5, 3, 4, 4, 5, 4, 5, 5, 6, 2, 3, 3, 4, 3, 4, 4, 5, 3, 4, 4, 5, 4, 5, 5, 6, 3, 4, 4, 5, 4, 5, 5, 6, 4, 5, 5, 6, 5,
	6, 6, 7, 2, 3, 3, 4, 3, 4, 4, 5, 3, 4, 4, 5, 4, 5, 5, 6, 3, 4, 4, 5, 4, 5, 5, 6, 4, 5, 5, 6, 5, 6, 6, 7, 3, 4, 4, 5,
	4, 5, 5, 6, 4, 5, 5, 6, 5, 6, 6, 7, 4, 5, 5, 6, 5, 6, 6, 7, 5, 6, 6, 7, 6, 7, 7, 8}

var emptyAmmo Ammo

func (a *Ammo) AckAdd() uint32 {
	return atomic.AddUint32(&a.ackCount, 1)
}

func (a *Ammo) ShootAdd() uint32 {
	return atomic.AddUint32(&a.shootCount, 1)
}

func (a *Ammo) ShootCount() uint32 {
	return atomic.LoadUint32(&a.shootCount)
}

func (a *Ammo) Free() {
	a.Id = 0
	a.Length = 0
	a.Kind = 0
	a.proof = 0
	a.ackCount = 0
	a.shootCount = 0
	a.Body = a.Body[:0]
}

func Unmarshal(b []byte) (Ammo, error) {

	if len(b) < 14 {
		return emptyAmmo, fmt.Errorf("missing length")
	}

	msg := Ammo{}
	msg.Length = binary.BigEndian.Uint32(b[:4])

	if int(msg.Length) != len(b)-4 {
		return emptyAmmo, fmt.Errorf("rogue")
	}

	msg.Id = binary.BigEndian.Uint32(b[4:8])
	msg.Kind = binary.BigEndian.Uint16(b[8:10])
	msg.proof = binary.BigEndian.Uint32(b[10:14])

	if len(b[14:]) > 0 {
		msg.Body = make([]byte, len(b[14:]))
	}
	//msg.Body = BytePool.Get().([]byte)[:len(b[14:])]
	copy(msg.Body, b[14:])

	var count uint32
	for i := 0; i < len(msg.Body); i++ {
		count += uint32(table[(msg.Body[i])])
	}

	if count != msg.proof {
		return emptyAmmo, fmt.Errorf("rogue")
	}

	return msg, nil
}

func Marshal(ammo Ammo) []byte {

	b := make([]byte, 14+len(ammo.Body))
	binary.BigEndian.PutUint32(b[:4], uint32(len(ammo.Body)+10))
	binary.BigEndian.PutUint32(b[4:8], ammo.Id)
	binary.BigEndian.PutUint16(b[8:10], ammo.Kind)
	var count uint32
	for i := 0; i < len(ammo.Body); i++ {
		count += uint32(table[(ammo.Body[i])])
	}
	binary.BigEndian.PutUint32(b[10:14], count)
	copy(b[14:], ammo.Body)
	return b
}
