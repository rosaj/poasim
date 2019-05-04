package main

import (
	"../config"
	"../network"
	"../plot"
	"../util"
	"fmt"
	"github.com/agoussia/godes"
	"math"
	"runtime"
	sysTime "time"
)


var startTime sysTime.Time


func runBootstrapNodes() []*network.Node {

	bootstrapNodes := make([]*network.Node, 3)

	for i := 0; i < len(bootstrapNodes); i++{
		bootstrapNodes[i] = network.NewBootstrapNode(bootstrapNodes)
	}

	for i := 0; i < len(bootstrapNodes); i++{
		godes.AddRunner(bootstrapNodes[i])
	}

	godes.Run()

	return bootstrapNodes
}

func createNodes(bootstrapNodes []*network.Node) []*network.Node  {

	nodes := make([]*network.Node, config.SimConfig.NodeCount)

	for i:=0 ; i < len(nodes); i++ {
		nodes[i] = network.NewNode(bootstrapNodes)
	}

	return nodes
}

func runNodes() []*network.Node {

	bNodes := runBootstrapNodes()

	nodes := createNodes(bNodes)


	for i:= 0; i < len(nodes) ; i++ {

		nodeArrival := config.SimConfig.NextNodeArrival()

		if nodeArrival > 0 {
			godes.Advance(nodeArrival)
		}

		godes.AddRunner(nodes[i])

		logProgress("Added node:", nodes[i])
	}

	logProgress("Added all nodes")

	return append(nodes, bNodes...)
}

func logProgress(a ...interface{})  {
	logging := config.LogConfig.Logging
	config.LogConfig.Logging = true

	util.Log(math.Round((godes.GetSystemTime()/config.SimConfig.SimulationTime)*100), "% elapsed:", sysTime.Since(startTime), a)

	config.LogConfig.Logging = logging
}

func runSim(){

	runtime.GOMAXPROCS(1)

	startTime = sysTime.Now()
	util.Log("start")

	nodes := runNodes()

	waitForSimEnd()

	if godes.GetSystemTime() > config.SimConfig.SimulationTime {
		config.SimConfig.SimulationTime = godes.GetSystemTime()
	}

	config.LogConfig.Logging = true

	util.Log("Simulation end after:", sysTime.Since(startTime))

	godes.Clear()

	showStats(nodes)

}

func waitForSimEnd()  {
	dif := config.SimConfig.SimulationTime - godes.GetSystemTime()

	if dif > 0 {

		if config.LogConfig.Logging {
			godes.Advance(dif)
		} else {

			chunks := 50
			part := dif / float64(chunks)

			for i := 1; i <= chunks; i++ {
				godes.Advance(part)
				logProgress()
			}
		}
	}
}


func showStats(nodes []*network.Node)  {

	totalSent := 0
	totalReceived := 0
	for _, node := range nodes {
		totalSent += node.GetTotalMessagesSent()
		totalReceived += node.GetTotalMessagesReceived()
	}

	fmt.Println("Sent [sum:", totalSent,"avg:", totalSent/len(nodes), "]")
	fmt.Println("Received [sum:", totalReceived,"avg:", totalReceived/len(nodes), "]")

	plot.Stats(nodes)
}


func main()  {
	runSim()
	//plot.Test()
}
