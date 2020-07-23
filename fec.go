package sharpshooter

import (
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/klauspost/reedsolomon"
)

type fecEncoder struct {
	dataShards int
	parShards  int
	enc        reedsolomon.Encoder
}

func (e *fecEncoder) encode(b []byte) ([][]byte, error) {

	//shards := make([][]byte, e.dataShards*e.parShards)
	//
	//for k := range shards {
	//	shards[k] = make([]byte, DEFAULT_INIT_PACKSIZE)
	//}
	//
	//for i := 0; int64(i) < e.dataShards; i++ {
	//
	//	if len(b) > DEFAULT_INIT_PACKSIZE {
	//
	//		copy(shards[i], b[:DEFAULT_INIT_PACKSIZE])
	//		b = b[DEFAULT_INIT_PACKSIZE:]
	//
	//	} else if len(b) != 0 {
	//		copy(shards[i], b)
	//	}
	//}
	des := make([]byte, len(b)+4)

	binary.BigEndian.PutUint32(des[0:4], uint32(len(b)))

	copy(des[4:], b)

	shards, err := e.enc.Split(des)
	if err != nil {
		return nil, err
	}

	err = e.enc.Encode(shards)
	if err != nil {
		return nil, err
	}

	return shards, nil

}

func newFecEncoder(dataShards, parShards int) *fecEncoder {
	fec := fecEncoder{
		dataShards: dataShards,
		parShards:  parShards,
	}

	fec.enc, _ = reedsolomon.New(dataShards, parShards)
	return &fec

}

type fecDecoder struct {
	dataShards int
	parShards  int
	dec        reedsolomon.Encoder
}

func newFecDecoder(dataShards, parShards int) *fecDecoder {
	fec := fecDecoder{
		dataShards: dataShards,
		parShards:  parShards,
	}

	fec.dec, _ = reedsolomon.New(dataShards, parShards)
	return &fec

}

func (d *fecDecoder) decode(b [][]byte) ([]byte, error) {

	buff := bytes.NewBuffer(nil)
	err := d.dec.Reconstruct(b)
	if err != nil {
		return nil, err
	}

	if len(b[0]) < 4 {
		return nil, errors.New("bad bytes")
	}

	lenght := binary.BigEndian.Uint32(b[0][:4])

	b[0] = b[0][4:]

	err = d.dec.Join(buff, b, int(lenght))
	if err != nil {
		return nil, err
	}

	return buff.Bytes(), nil

}
