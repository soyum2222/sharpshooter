package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"sharpshooter"
	"time"
)

func main() {
	go http.ListenAndServe(":8888", nil)

	begin := time.Now()

	conn, err := sharpshooter.Dial(":8890")
	if err != nil {
		println(err)
	}
	conn.OpenDeferSend()

	info, err := os.Stat("./source")
	if err != nil {
		panic(err)
	}
	size := info.Size()

	file, err := os.Open("./source")

	if err != nil {
		panic(err)
	}
	b := make([]byte, 1024)
	var count int64
	for i := 0; ; i++ {

		n, err := file.Read(b)
		if err != nil {
			//panic(err)
			fmt.Println(err)
			break
		}
		count += int64(n)
		_, err = conn.Write(b[:n])
		if err != nil {
			panic(err)
		}

		//print planned speed
		if i%100 == 0 {
			fmt.Println(float64(count) / float64(size))
		}

	}

	//conn.Write(b)
	conn.Close()

	end := time.Now()

	fmt.Println(end.Unix() - begin.Unix())
	fmt.Println(sharpshooter.Flow)
}