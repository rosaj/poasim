package util

import (
	"fmt"
	"github.com/agoussia/godes"
	"math"
)

func Wait(time float64)  {
	bControl := godes.NewBooleanControl()
	bControl.Set(false)

	bControl.WaitAndTimeout(true,time )

}

func Log(a ...interface{}){
	sysTime := godes.GetSystemTime()
	seconds := math.Mod(sysTime, 60)
	minutes := math.Mod(sysTime/60, 60)
	hours := math.Mod(sysTime/3600, 60)
	fmt.Printf("%d:%d:%d \t %v \n", int(hours), int(minutes), int(seconds), a)

	//fmt.Println(godes.GetSystemTime(), a)
}
