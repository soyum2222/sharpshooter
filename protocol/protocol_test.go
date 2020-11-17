package protocol

import (
	"encoding/binary"
	"testing"
)

func compAmmo(a1, a2 Ammo) bool {

	if a1.Length != a2.Length || a1.Id != a2.Id || a1.Kind != a2.Kind {
		return false
	}

	for len(a1.Body) != len(a2.Body) {
		return false
	}

	for i := 0; i < len(a1.Body); i++ {

		for j := i; j < len(a2.Body); j++ {
			if a1.Body[i] != a2.Body[j] {
				return false
			}
			continue
		}
	}

	return true
}

func TestMarshalUnmarshal(t *testing.T) {
	ammo := Ammo{
		Length: 10,
		Id:     1,
		Kind:   1,
		Body:   make([]byte, 4),
	}

	ab := Marshal(ammo)

	newAmmo, err := Unmarshal(ab)
	if err != nil {
		t.Fail()
		return
	}

	if !compAmmo(ammo, newAmmo) {
		t.Fail()
		return
	}

	b := make([]byte, 0)

	ammo, err = Unmarshal(b)
	if err == nil {
		t.Fail()
		return
	}
}

func TestRogue(t *testing.T) {

	ammo := Ammo{
		Id:   1,
		Kind: 1,
		Body: make([]byte, 10),
	}

	b := Marshal(ammo)

	binary.BigEndian.PutUint32(b[0:4], 10)
	_, err := Unmarshal(b)
	if err == nil {
		t.Fail()
	}

	binary.BigEndian.PutUint32(b[0:4], 5)
	_, err = Unmarshal(b)
	if err == nil {
		t.Fail()
	}

	binary.BigEndian.PutUint32(b[0:4], 16)
	_, err = Unmarshal(b)
	if err != nil {
		t.Fail()
	}

}

func BenchmarkUnmarshal(b *testing.B) {

	for i := 0; i < b.N; i++ {
		b := make([]byte, 1024)

		for k := range b {
			b[k] = uint8(k)
		}

		Unmarshal(b)
	}
}

func BenchmarkMarshal(b *testing.B) {

	for i := 0; i < b.N; i++ {
		b := make([]byte, 1024)

		for k := range b {
			b[k] = uint8(k)
		}

		Marshal(Ammo{
			Body: b,
		})
	}
}
