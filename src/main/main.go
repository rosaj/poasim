package main

import (
	"../network"
	"../util"
	"fmt"
	"github.com/agoussia/godes"
	"runtime"
	sysTime "time"
)



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
	runtime.GOMAXPROCS(1)

	start:= sysTime.Now()

	nodes := make([]*network.Node, 3)
	godes.Run()
	for i:=0 ; i < len(nodes); i++{
		node := network.NewNode(func(node *network.Node) {
					for{
						util.Wait(10)
						for _, n := range nodes {
							if n != node{
								node.Ping(n)
							}
						}
					}
				})
		nodes[i] = node
		godes.AddRunner(node)
	}


	util.Wait(30)
	godes.Clear()
	elapsed := sysTime.Since(start)
	fmt.Println(elapsed)

	//godes.WaitUntilDone()

/*
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
*/
}