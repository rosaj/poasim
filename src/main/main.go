package main

import (
	. "../common"
	"../config"
	"../export"
	"../generate"
	"../network"
	"../network/eth/core"
	"../network/protocol"
	. "../scenario"
	"../util"
	"github.com/agoussia/godes"
	"math"
	"runtime"
	sysTime "time"
)


var startTime sysTime.Time

var c = 0

func newNodeConfig(bootNodes []*network.Node) *network.NodeConfig {


	protocols := make([]string, 0)
	protocols = append(protocols, protocol.ETH)
	networkId := 1

/*
	c++
	if (c == 56) || (c == 64) || (c == 84) {
		networkId = 3
	}
*/

	return &network.NodeConfig{
		BootNodes: bootNodes,
		MaxPeers: config.SimConfig.MaxPeers,
		DialRatio: config.SimConfig.DialRatio,
		Protocols: protocols,
		NetworkID: networkId,
		EthereumConfig: &config.EthConfig,
	}
}
var bootNodeCount = 1
func runBootstrapNodes() []*network.Node {

	bootNodes := make([]*network.Node, bootNodeCount)

	for i := 0; i < len(bootNodes); i++{
		bootNodes[i] = network.NewBootstrapNode(newNodeConfig(bootNodes))
		bootNodes[i].NetworkID = 3
	}

	for i := 0; i < len(bootNodes); i++{
		godes.AddRunner(bootNodes[i])
	}

	godes.Run()

	return bootNodes
}

func createNodes(bootNodes []*network.Node, count int) []*network.Node {
	core.Sealers = make([]INode, 0)

	nodes := make([]*network.Node, count)

	for i:=0 ; i < len(nodes); i++ {
		nodes[i] = network.NewNode(newNodeConfig(bootNodes))
		core.Sealers = append(core.Sealers, nodes[i])
	}

	return nodes
}

func createSimNodes(bootNodes []*network.Node) []*network.Node  {
	return createNodes(bootNodes, config.SimConfig.NodeCount)
}

func runNodes() []*network.Node {

	bNodes := runBootstrapNodes()

	nodes := createSimNodes(bNodes)


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
	util.Print(math.Round((godes.GetSystemTime()/config.SimConfig.SimulationTime)*100), "% elapsed:", sysTime.Since(startTime), a)
}
func runSim(){

	runtime.GOMAXPROCS(1)

	startTime = sysTime.Now()
	util.Log("start")

	nodes := runNodes()

	godes.Advance(config.SimConfig.NodeStabilisationTime)

	godes.Advance(3 * 60)

	//nodes[3].Kill()

	if config.SimConfig.SimMode == config.BLOCKCHAIN {
		generate.AsyncTxsDistr(nodes[:config.SimConfig.NodeCount])
	}


//	ScenarioNodeLeavingNetwork(nodes[:len(nodes) - bootNodeCount],  config.SimConfig.NodeCount/2, sysTime.Hour)




	waitForEnd(nodes)
}


func progressSimToEnd()  {
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



func waitForEnd(nodes []*network.Node) {

	progressSimToEnd()

	if godes.GetSystemTime() > config.SimConfig.SimulationTime {
		config.SimConfig.SimulationTime = godes.GetSystemTime()
	}

	config.LogConfig.Logging = true

	util.Log("Simulation end after:", sysTime.Since(startTime))

	godes.Clear()

	showStats(nodes)
	//showStats(nodes[0:len(nodes) - bootNodeCount ])

}

func showStats(nodes []*network.Node)  {
	export.Stats(nodes)

	if config.SimConfig.SimMode == config.BLOCKCHAIN {
		PrintBlockchainStats(nodes)
	}
}


func main()  {
	runSim()
	//export.Test()
}
