package main

import (
	"sharpshooter"
)

func main() {

	conn, err := sharpshooter.Dial("127.0.0.1:8858")
	if err != nil {
		panic(err)
	}

	b := make([]byte, 1024)
	var i int
	for {

		_, err := conn.Read(b)
		if err != nil {
			panic(err)
		}
		i++
		//fmt.Println(i)
	}

}
