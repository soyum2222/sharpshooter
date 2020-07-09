package main

import (
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"sharpshooter"
)

func main() {

	go http.ListenAndServe(":9999", nil)

	addr := &net.UDPAddr{
		IP:   nil,
		Port: 8858,
		Zone: "",
	}
	h, err := sharpshooter.Listen(addr)
	if err != nil {
		panic(err)
	}

	sniper, err := h.Accept()
	if err != nil {
		panic(err)
	}

	file, err := os.Create("test")
	if err != nil {
		panic(err)
	}

	b := make([]byte, 10240)
	for i := 0; ; i++ {
		n, err := sniper.Read(b)
		if err != nil {
			fmt.Println(err)
			break
		}
		_, err = file.Write(b[:n])
		if err != nil {
			panic(err)
		}
	}

	file.Close()

}
