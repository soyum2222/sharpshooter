package protocol

import (
	"encoding/binary"
	"fmt"
	"sync"
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

var BytePool sync.Pool

func init() {
	BytePool.New = func() interface{} {
		return make([]byte, 2048)
	}
}

type Ammo struct {
	Length   uint32
	Id       uint32
	Kind     uint16
	proof    uint32
	Body     []byte
	ackCount uint32
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

	//msg.Body = make([]byte, 0, len(b[10:]))
	msg.Body = BytePool.Get().([]byte)[:len(b[14:])]
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

	b := BytePool.Get().([]byte)[:14+len(ammo.Body)]
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
