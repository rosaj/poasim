package main

import (
	"../network"
	"../util"
	"fmt"
	"github.com/agoussia/godes"
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
	trAdded.Set(false)
	nodes := make([]*network.Node, 3)
	godes.Run()
	for i:=0 ; i < 3; i++{
		node := network.NewNode(func(node *network.Node) {
					for{
						trAdded.WaitAndTimeout(true, 10)
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


	util.Wait(60)
	godes.Clear()

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


func Log(a ...interface{}){
	fmt.Println(godes.GetSystemTime(), a)
}