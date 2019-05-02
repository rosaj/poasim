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
		fmt.Println(math.Round(godes.GetSystemTime()/config.SimConfig.SimulationTime), "% - Added node:", nodes[i])
	}

	fmt.Println("Added all nodes")
	return append(nodes, bNodes...)
}

func runSim(){

	runtime.GOMAXPROCS(1)

	startTime:= sysTime.Now()

	util.Log("start")


	nodes := runNodes()


	waitForSimEnd(startTime)

	if godes.GetSystemTime() > config.SimConfig.SimulationTime {
		config.SimConfig.SimulationTime = godes.GetSystemTime()
	}

	config.LogConfig.Logging = true

	elapsed := sysTime.Since(startTime)
	util.Log("Simulation end after:", elapsed)

	godes.Clear()

	showStats(nodes)

}

func waitForSimEnd(startTime sysTime.Time)  {
	dif := config.SimConfig.SimulationTime - godes.GetSystemTime()

	if dif > 0 {

		if config.LogConfig.Logging {
			godes.Advance(dif)
		} else {

			initialPct := math.Floor((godes.GetSystemTime()/config.SimConfig.SimulationTime)*100)/100

			chunks := 100
			part := dif / float64(chunks)

			for i := 1; i <= chunks; i++ {
				godes.Advance(part)

				config.LogConfig.Logging = true

				elapsed := sysTime.Since(startTime)

				percentage := math.Round( ( initialPct + (float64(i)*part)/config.SimConfig.SimulationTime)  * 100)
				//percentage := math.Floor((((float64(i)*part)/dif )/(1-initialPct) - initialPct ) *100)

				util.Log(percentage, "%: elapsed:", elapsed)

				if percentage < 98 {
					config.LogConfig.Logging = false
				}
			}
		}
	}
}


func showStats(nodes []*network.Node)  {

	sum := 0
	for _, node := range nodes {
		totalSent := node.GetTotalMessagesSent()
		sum += totalSent
	}

	fmt.Println("Sum sent", sum)
	fmt.Println("Average sent", sum/len(nodes))
	plot.Stats(nodes)
}


func main()  {
	runSim()
	//plot.Test()
}
