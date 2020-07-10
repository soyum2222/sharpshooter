package tool

import (
	"fmt"
	"runtime"
	"time"
)

func TimeConsuming() func() {

	pc, _, _, _ := runtime.Caller(1)

	f := runtime.FuncForPC(pc)

	t := time.Now().UnixNano()
	return func() {
		fmt.Printf("%s time consuming : %d \n", f.Name(), time.Now().UnixNano()-t)
	}

}
