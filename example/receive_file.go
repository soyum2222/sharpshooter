package main

import (
	"fmt"
	"github.com/soyum2222/sharpshooter"
	"net/http"
	_ "net/http/pprof"
	"os"
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
