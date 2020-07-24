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
	buff       []byte
}

func (e *fecEncoder) encode(b []byte) ([][]byte, error) {

	if cap(e.buff) < len(b) {
		e.buff = make([]byte, 4, len(b))
	}

	binary.BigEndian.PutUint32(e.buff[0:4], uint32(len(b)))

	e.buff = append(e.buff, b...)

	// TODO here can be optimization
	shards, err := e.enc.Split(e.buff)
	if err != nil {
		return nil, err
	}

	err = e.enc.Encode(shards)
	if err != nil {
		return nil, err
	}

	e.buff = e.buff[:4]

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
	buff       *bytes.Buffer
}

func newFecDecoder(dataShards, parShards int) *fecDecoder {
	fec := fecDecoder{
		dataShards: dataShards,
		parShards:  parShards,
		buff:       bytes.NewBuffer(nil),
	}

	fec.dec, _ = reedsolomon.New(dataShards, parShards)
	return &fec

}

func (d *fecDecoder) decode(b [][]byte) ([]byte, error) {

	defer d.buff.Reset()

	err := d.dec.Reconstruct(b)
	if err != nil {
		return nil, err
	}

	if len(b[0]) < 4 {
		return nil, errors.New("bad bytes")
	}

	length := binary.BigEndian.Uint32(b[0][:4])

	b[0] = b[0][4:]

	err = d.dec.Join(d.buff, b, int(length))
	if err != nil {
		return nil, err
	}

	return d.buff.Bytes(), nil
}
