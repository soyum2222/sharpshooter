package protocol

import (
	"encoding/binary"
	"sync/atomic"
)

const (
	ACK = iota
	FIRSTHANDSHACK
	SECONDHANDSHACK
	THIRDHANDSHACK
)

type Ammo struct {
	Length   uint32
	Id       uint32
	Kind     uint16
	Body     []byte
	ackCount uint32
}

func (a *Ammo) AckAdd() uint32 {
	return atomic.AddUint32(&a.ackCount, 1)
}

func Unmarshal(b []byte) Ammo {

	msg := Ammo{}
	msg.Length = binary.BigEndian.Uint32(b[:4])
	msg.Id = binary.BigEndian.Uint32(b[4:8])
	msg.Kind = binary.BigEndian.Uint16(b[8:10])
	msg.Body = make([]byte, len(b[10:]))
	copy(msg.Body, b[10:])

	return msg

}

func Marshal(ammo Ammo) []byte {

	b := make([]byte, 10+len(ammo.Body))

	binary.BigEndian.PutUint32(b[:4], uint32(len(ammo.Body)+6))
	binary.BigEndian.PutUint32(b[4:8], ammo.Id)
	binary.BigEndian.PutUint16(b[8:10], ammo.Kind)
	copy(b[10:], ammo.Body)
	return b

}
