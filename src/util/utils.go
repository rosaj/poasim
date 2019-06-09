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
func LogError(err error) {
	Print(err)
}

func Print(a ...interface{})  {
	fmt.Println(ToDuration(godes.GetSystemTime()), a)
}

func ToDuration(seconds float64)  time.Duration {
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

func StartNewRunner(runFn func()) godes.Runner {
	runner := &tempRunner{&godes.Runner{}, runFn}
	godes.AddRunner(runner)
	return *runner.Runner
}

type tempRunner struct {
	*godes.Runner
	runFn func()
}

func (t *tempRunner) Run()  {
	t.runFn()
}