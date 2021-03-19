package main

import (
	"github.com/soyum2222/sharpshooter"
	"io"
	"os"
)

func main() {

	l, err := sharpshooter.Listen(":8858")
	if err != nil {
		panic(err)
	}

	conn, err := l.Accept()
	if err != nil {
		panic(err)
	}
	//conn.OpenStaTraffic()

	file, err := os.Open("./test")
	if err != nil {
		panic(err)
	}

	_, err = io.Copy(conn, file)
	if err != nil {
		panic(err)
	}

	//fmt.Println(conn.TrafficStatistics())
}
