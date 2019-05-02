package util

import (
	"../config"
	"fmt"
	"github.com/agoussia/godes"
	"time"
)


func Log(a ...interface{}){

	if !config.LogConfig.Logging {
		return
	}

	fmt.Println(ToDuration(godes.GetSystemTime()), a)
}

func ToDuration(seconds float64)  time.Duration {

	t := time.Now()
	t1 := t.Add(time.Second * time.Duration(seconds))
	dif := t1.Sub(t)
	return dif
}