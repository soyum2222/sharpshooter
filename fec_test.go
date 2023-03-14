package sharpshooter

import (
	"fmt"
	"testing"
)

func TestFecEncode(t *testing.T) {

	b := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	e := newFecEncoder(4, 2)

	bs, err := e.encode(b)
	if err != nil {
		fmt.Println(err)
		t.Fail()
		return
	}

	fmt.Println(bs)

	d := newFecDecoder(4, 2)

	bs[1] = nil
	bs[2] = nil
	b, err = d.decode(bs)
	if err != nil {
		t.Fail()
		return
	}
}
