package main

import (
	"fmt"
	"github.com/agoussia/godes"
)


var config = Test

var nodes *Nodes



var trQueue = godes.NewFIFOQueue("0")
var trAdded = godes.NewBooleanControl()



type Nodes  struct {
	count int
}

type Transaction struct{
	num int
}



func main(){

	nodes = &Nodes{config.ValidatorCount}

	trAdded.Set(false)
	godes.Run()

	for i := 0; i < nodes.count; i++ {
		node := NewNode()

		godes.AddRunner(node)
	}


	counter := 1


	for {

		trQueue.Place(&Transaction{counter})
		trAdded.Set(true)
		godes.Advance(config.nextTrInterval())

		counter += 1

		if godes.GetSystemTime() > config.SimulationTime {
			break
		}

	}

	godes.Clear()
	//godes.WaitUntilDone()
}

func addTx(){



}

func Log(a ...interface{}){
	fmt.Println(godes.GetSystemTime(), a)
}