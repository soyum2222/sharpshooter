package protocol

import (
	"encoding/binary"
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
	OUTOFAMMO
)

type Ammo struct {
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
	msg.Id = binary.BigEndian.Uint32(b[0:4])
	msg.Kind = binary.BigEndian.Uint16(b[4:6])
	msg.Body = make([]byte, 0, len(b[6:]))

	msg.Body = append(msg.Body, b[6:]...)
	return msg

}

func Marshal(ammo Ammo) []byte {

	b := make([]byte, 6, 6+len(ammo.Body))

	binary.BigEndian.PutUint32(b[:4], ammo.Id)
	binary.BigEndian.PutUint16(b[4:6], ammo.Kind)

	b = append(b, ammo.Body...)
	return b

}
