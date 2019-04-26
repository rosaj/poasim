package util

import (
	"fmt"
	"github.com/agoussia/godes"
	"time"
)

func Wait(time float64)  {
	bControl := godes.NewBooleanControl()
	bControl.Set(false)

	bControl.WaitAndTimeout(true,time )

}

func Log(a ...interface{}){

	sysTime := godes.GetSystemTime()

	t := time.Now()
	t1 := t.Add(time.Second * time.Duration(sysTime))
	dif := t1.Sub(t)

	fmt.Println(dif, a)

}

