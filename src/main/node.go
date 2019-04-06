package main

import (
	"github.com/agoussia/godes"
)

var nodeCounter = 1

type Node struct {
	*godes.Runner
	id int
}

func NewNode() (node *Node){

	node = &Node{&godes.Runner{}, nodeCounter}
	nodeCounter += 1

	return
}
var time = godes.GetSystemTime()

func (node *Node) Run(){

	for {

		if godes.GetSystemTime() - time >= config.BlockTime {
			time = godes.GetSystemTime()
			node.forwardMessageToPeers()
			Log("Block sent by ", node.id)
		}else {
			trAdded.WaitAndTimeout(true,  config.BlockTime)

			if trAdded.GetState(){
				trAdded.Set(false)
				trNum := trQueue.Get().(*Transaction).num
				Log("Transaction send: ", trNum, " by ", node.id)
			}
			/*

			if trQueue.Len() > 0 {
				node.forwardMessageToPeers()


				tr := trQueue.Get()
				if tr != nil{
					trNum := tr.(*Transaction).num
					Log("Transaction send: ", trNum)
				}

			}
			*/

		}

	}
}

func (node *Node) forwardMessageToPeers(){
	max := config.nextNodeInterval()

	for i := 0; i < config.NodeCount - 1; i++ {
		if interval := config.nextNodeInterval(); interval > max{
			max = interval
		}

	}
//	Log("forwardMessageToPeers ", max)
	godes.Advance(max)
}

