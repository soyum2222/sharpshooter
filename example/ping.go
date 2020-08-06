package main

import (
	"encoding/binary"
	"fmt"
	"github.com/soyum2222/sharpshooter"
	"io"
	"net/http"
	_ "net/http/pprof"
	"time"
)

func main() {

	go http.ListenAndServe(":8888", nil)

	conn, err := sharpshooter.Dial("127.0.0.1:8858")
	if err != nil {
		panic(err)
	}

	s := "ping"
	lenght := make([]byte, 4)
	for {

		binary.BigEndian.PutUint32(lenght, uint32(len(s)))

		_, err = conn.Write(append(lenght, []byte(s)...))
		if err != nil {
			panic(err)
		}

		_, err := conn.Read(lenght)
		if err != nil {
			panic(err)
		}

		b := make([]byte, binary.BigEndian.Uint32(lenght))

		_, err = io.ReadFull(conn, b)
		if err != nil {
			panic(err)
		}

		fmt.Println(string(b))

		time.Sleep(time.Second)
	}

}
