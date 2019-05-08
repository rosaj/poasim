package main

import (
	"../config"
	"../network"
	"../plot"
	"../util"
	"fmt"
	"github.com/agoussia/godes"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"math"
	"math/big"
	"math/rand"
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

func createNodes(bootstrapNodes []*network.Node, count int) []*network.Node {
	nodes := make([]*network.Node, count)

	for i:=0 ; i < len(nodes); i++ {
		nodes[i] = network.NewNode(bootstrapNodes)
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

	godes.Advance(5 * 60)



	interval := 1.0
	times := 1000
	for times > 0 {
		times-=1
		godes.Advance(interval)
		interval+=0.1

		txs := make([]*types.Transaction,0)

		tx := types.NewTransaction(uint64(times), common.Address{}, big.NewInt(1000), 10000, big.NewInt(1999), nil)
		txs = append(txs, tx)
		nodes[rand.Intn(config.SimConfig.NodeCount)].Server().ProtocolManager().BroadcastTxs(txs)
	}






	/*


	godes.Advance(config.SimConfig.NodeStabilisationTime)

	deadNodes := 500
	temp := nodes[:deadNodes]

	for _, node := range temp {
		node.Kill()
	}

	godes.Advance(config.SimConfig.NodeStabilisationTime)
	temp = nodes[deadNodes+1:800]

	for _, node := range temp {
		node.Kill()
	}

	godes.Advance(config.SimConfig.NodeStabilisationTime)



	temp = createNodes(nodes[config.SimConfig.NodeCount:], deadNodes)
	for _, node := range temp {
		godes.AddRunner(node)
		logProgress("Added new node:", node)
	}
	nodes = append(nodes, temp...)


*/


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
