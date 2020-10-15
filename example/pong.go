package main

import (
	"encoding/binary"
	"fmt"
	"github.com/soyum2222/sharpshooter"
	"io"
	"net/http"
	_ "net/http/pprof"
)

func main() {

	go http.ListenAndServe(":9999", nil)


	h, err := sharpshooter.Listen(":8858")
	if err != nil {
		panic(err)
	}

	sniper, err := h.Accept()
	if err != nil {
		panic(err)
	}

	s := "pong"

	lenght := make([]byte, 4)
	for {

		_, err := sniper.Read(lenght)
		if err != nil {
			fmt.Println(err)
		}

		b := make([]byte, binary.BigEndian.Uint32(lenght))

		_, err = io.ReadFull(sniper, b)
		if err != nil {
			panic(err)
		}

		fmt.Println(string(b))

		binary.BigEndian.PutUint32(lenght, uint32(len(s)))

		_, err = sniper.Write(append(lenght, []byte(s)...))
		if err != nil {
			panic(err)
		}
	}
}
