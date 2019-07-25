package main

import "C"

import (
	. "../common"
	"../config"
	"../export"
	"../generate"
	"../network"
	"../network/eth"
	"../network/eth/core"
	"../network/protocol"
	. "../scenario"
	"../util"
	"fmt"
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

//export RunSim
func RunSim(nodeCount int, blockTime int, maxPeers int, consensus int) (float64, float64) {

	FinalityMetricCollector.Reset()
	generate.Reset()
	network.Reset()

	config.SimConfig.NodeCount = nodeCount
	config.SimConfig.MaxPeers = maxPeers
	config.ChainConfig.Clique.Period = uint64(blockTime)
	config.ChainConfig.Aura.Period = uint64(blockTime)

	config.MetricConfig.ExportType = config.NA

	config.ChainConfig.Engine = config.ConsensusEngine(consensus)

	return StartSim()
}



func StartSim() (float64, float64) {

	runtime.GOMAXPROCS(1)

	startTime = sysTime.Now()
	util.Log("start")

	nodes := runNodes()

	godes.Advance(config.SimConfig.NodeStabilisationTime)

	logProgress("Starting minting")
	startMinting(nodes)

	godes.Advance(config.SimConfig.NodeStabilisationTime)

	if config.SimConfig.SimMode == config.BLOCKCHAIN {
		logProgress("Tx arrival")
		generate.AsyncTxsDistr(nodes[:config.SimConfig.NodeCount])
	}

	//godes.Advance(config.SimConfig.SimulationTime - godes.GetSystemTime() - 10)
	//printDEVp2pConnections(nodes)
//	ScenarioNodeLeavingNetwork(nodes[:len(nodes) - bootNodeCount],  config.SimConfig.NodeCount/2, sysTime.Hour)

	waitForEnd(nodes)

	return calcFinality()
}

func startMinting(nodes []*network.Node)  {

	for _, node := range nodes {
		if srv := node.Server(); srv != nil {
			e := srv.(*eth.Ethereum)
			ebase, _ := e.Etherbase()
			e.Miner().Start(ebase)
		}
	}

}

func calcFinality() (float64, float64)  {
//	t := make(map[int]int)
	w, f := 0.0, 0.0
	for _, tx := range FinalityMetricCollector.Collect() {
		waitTime, finality := tx.CalcFinality()
		w+=waitTime
		f+=finality
		//	t[len(tx.Inserted)]+=1
		if len(tx.Forked) == 0 || len(tx.Inserted) == config.SimConfig.NodeCount {
			continue
		}/*
		util.Print("Submited", tx.Submitted)
		util.Print("Included", tx.Included)
		util.Print("Inserted", tx.Inserted)
		util.Print("Forked", tx.Forked)
		util.Print("")*/
	}

	count := len(FinalityMetricCollector.Collect())

	//util.Print(w/float64(count), f/float64(count))

	return w/float64(count), f/float64(count)
	/*
		for k, v := range t {
			util.Print(k, v)
		}*/
}

// N = 6

// AURA
// [10.051947452134614 0.40547179307658965]
// [13.949590988502898 0.4003330614381889]
// [10.129061363800826 0.40334000143182525]

// CLIQUE
// [10.049930785103806 0.8499236772946238]
// [10.072293669547486 0.7186380174758570]
// [9.944465135962895  0.8539474318580816]


// N = 60

// AURA
//


// CLIQUE
//

func printDEVp2pConnections(nodes []*network.Node)  {

	for _, node := range nodes {
		util.Print(node.Name())
		if node.Server() == nil {
			continue
		}
		for _, peer := range node.GetDEVp2pPeers() {
			fmt.Print(peer.Name(), ", ")
		}
		fmt.Println("")
	}
}

func progressSimToEnd()  {
	//loggingProgress := true

	dif := config.SimConfig.SimulationTime - godes.GetSystemTime()

	if dif > 0 {

		if config.LogConfig.Logging /*|| !loggingProgress*/ {
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

	//config.LogConfig.Logging = true

	util.Log("Simulation end after:", sysTime.Since(startTime))

	godes.Clear()

	//showStats(nodes)
	//showStats(nodes[0:len(nodes) - bootNodeCount ])

}

func showStats(nodes []*network.Node)  {
	export.Stats(nodes)

	if config.SimConfig.SimMode == config.BLOCKCHAIN {
		PrintBlockchainStats(nodes, false)
	}
}


func main() {
	//RunSim(71, 20, 19, 1)
	StartSim()
	//RunSim(5, 15)
}


//go build -buildmode=c-shared -o poasim.so  src/main/main.go