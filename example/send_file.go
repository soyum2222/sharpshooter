package main

import (
	"fmt"
	"github.com/soyum2222/sharpshooter"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"
)

func main() {
	go http.ListenAndServe(":8888", nil)

	begin := time.Now()

	conn, err := sharpshooter.Dial("127.0.0.1:8858")
	if err != nil {
		panic(err)
	}
	//conn.OpenStaFlow()
	//conn.OpenStaFlow()

	info, err := os.Stat("./source")
	if err != nil {
		panic(err)
	}
	size := info.Size()
	fmt.Println(size)

	file, err := os.Open("./source")

	if err != nil {
		panic(err)
	}
	b := make([]byte, 1024)
	var count int64
	for i := 0; ; i++ {

		n, err := file.Read(b)
		if err != nil {
			fmt.Println(err)
			break
		}
		count += int64(n)
		_, err = conn.Write(b[:n])
		if err != nil {
			panic(err)
		}

		//print planned speed
		if i%1000 == 0 {
			fmt.Println(float64(count) / float64(size))
		}
	}

	time.Sleep(time.Second * 5)
	conn.Close()

	end := time.Now()

	fmt.Println(end.Unix() - begin.Unix())
}
