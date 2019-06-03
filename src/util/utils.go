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
	Print(a)
}

func Print(a ...interface{})  {
	fmt.Println(ToDuration(godes.GetSystemTime()), a)
}

func ToDuration(seconds float64)  time.Duration {

	/*
	t := time.Now()
	t1 := t.Add(time.Second * time.Duration(seconds))
	dif := t1.Sub(t)
	 */

	return time.Duration(seconds) * time.Second
}

func TimeSince(time uint64) time.Duration {
	return ToDuration(godes.GetSystemTime() - float64(time))
}

func SecondsSince(time uint64) uint64 {
	return uint64(TimeSince(time).Seconds())
}

func SecondsNow() uint64 {
	return uint64(godes.GetSystemTime())
}

func LogError(err error) {
	Print(err)
}