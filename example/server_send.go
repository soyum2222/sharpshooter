package main

import (
	"fmt"
	"github.com/soyum2222/sharpshooter"
	"io"
	"net"
	"os"
)

func main() {

	addr := &net.UDPAddr{
		IP:   nil,
		Port: 8858,
		Zone: "",
	}
	l, err := sharpshooter.Listen(addr)
	if err != nil {
		panic(err)
	}

	conn, err := l.Accept()
	if err != nil {
		panic(err)
	}
	conn.OpenStaFlow()

	file, err := os.Open("./test")
	if err != nil {
		panic(err)
	}

	_, err = io.Copy(conn, file)
	if err != nil {
		panic(err)
	}

	fmt.Println(conn.FlowStatistics())

}
