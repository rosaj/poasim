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

func newNodeConfig(bootstrapNodes []*network.Node) *network.NodeConfig {


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
		BootstrapNodes: bootstrapNodes,
		MaxPeers: config.SimConfig.MaxPeers,
		Protocols: protocols,
		NetworkID: networkId,
		EthereumConfig: &config.EthConfig,
	}
}
var bootstrapNodeCount = 1
func runBootstrapNodes() []*network.Node {

	bootstrapNodes := make([]*network.Node, bootstrapNodeCount)

	for i := 0; i < len(bootstrapNodes); i++{
		bootstrapNodes[i] = network.NewBootstrapNode(newNodeConfig(bootstrapNodes))
		bootstrapNodes[i].NetworkID = 3
	}

	for i := 0; i < len(bootstrapNodes); i++{
		godes.AddRunner(bootstrapNodes[i])
	}

	godes.Run()

	return bootstrapNodes
}

func createNodes(bootstrapNodes []*network.Node, count int) []*network.Node {
	core.Sealers = make([]INode, 0)

	nodes := make([]*network.Node, count)

	for i:=0 ; i < len(nodes); i++ {
		nodes[i] = network.NewNode(newNodeConfig(bootstrapNodes))
		core.Sealers = append(core.Sealers, nodes[i])
	}

	return nodes
}

func createSimNodes(bootstrapNodes []*network.Node) []*network.Node  {
	return createNodes(bootstrapNodes, config.SimConfig.NodeCount)
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

	if config.SimConfig.SimMode == config.BLOCKCHAIN {

//		generate.AsyncTxs(nodes[:config.SimConfig.NodeCount], 1000, 36000, 0.08)

		generate.AsyncTxsDistr(nodes[:config.SimConfig.NodeCount])


	}


//	ScenarioNodeLeavingNetwork(nodes[:len(nodes) - bootstrapNodeCount],  config.SimConfig.NodeCount/2, sysTime.Hour)




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
	//showStats(nodes[0:len(nodes) - bootstrapNodeCount ])

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
