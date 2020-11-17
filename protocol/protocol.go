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

type Ammo struct {
	Length   uint32
	Id       uint32
	Kind     uint16
	Body     []byte
	ackCount uint32
}

var emptyAmmo Ammo

func (a *Ammo) AckAdd() uint32 {
	return atomic.AddUint32(&a.ackCount, 1)
}

func Unmarshal(b []byte) (Ammo, error) {

	if len(b) < 10 {
		return emptyAmmo, fmt.Errorf("missing length")
	}

	msg := Ammo{}
	msg.Length = binary.BigEndian.Uint32(b[:4])

	if int(msg.Length) != len(b)-4 {
		return emptyAmmo, fmt.Errorf("rogue")
	}

	msg.Id = binary.BigEndian.Uint32(b[4:8])
	msg.Kind = binary.BigEndian.Uint16(b[8:10])
	msg.Body = make([]byte, 0, len(b[10:]))
	msg.Body = append(msg.Body, b[10:]...)
	return msg, nil
}

func Marshal(ammo Ammo) []byte {
	b := make([]byte, 10, 10+len(ammo.Body))
	binary.BigEndian.PutUint32(b[:4], uint32(len(ammo.Body)+6))
	binary.BigEndian.PutUint32(b[4:8], ammo.Id)
	binary.BigEndian.PutUint16(b[8:10], ammo.Kind)
	b = append(b, ammo.Body...)
	return b
}
