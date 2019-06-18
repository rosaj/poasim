package main

import (
	. "../common"
	"../config"
	"../export"
	"../generate"
	"../network"
	"../network/eth"
	"../network/eth/common"
	"../network/eth/core"
	"../network/protocol"
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

func runBootstrapNodes() []*network.Node {

	bootstrapNodes := make([]*network.Node, 1)

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

	if config.SimConfig.SimMode == config.ETHEREUM {

		generate.AsyncTxs(nodes[:config.SimConfig.NodeCount], 1000, 36000, 0.08)

	//	generate.TxsDistr(nodes[:config.SimConfig.NodeCount], 1000)


	}
	//TODO: metrike npr. blockchain broj insert-a, forka, sidechaina, txs received etc

	/*

	generate.AsyncTxs(nodes[:config.SimConfig.NodeCount], 1000, 180, 0.05)
	godes.Advance(15)
	generate.AsyncTxs(nodes[:config.SimConfig.NodeCount], 1000, 180, 0.05)
	godes.Advance(15)
	generate.AsyncTxs(nodes[:config.SimConfig.NodeCount], 1000, 180, 0.05)

	//godes.Advance(15)

	generate.AsyncTxs(nodes[:config.SimConfig.NodeCount], 1000, 100, 0.02)
	generate.AsyncTxs(nodes[:config.SimConfig.NodeCount], 1000, 100, 0.03)
	generate.AsyncTxs(nodes[:config.SimConfig.NodeCount], 1000, 100, 0.04)
	generate.AsyncTxs(nodes[:config.SimConfig.NodeCount], 1000, 100, 0.05)

	/*
	 for !config.SimConfig.SimulationEnded() {
	 	  godes.Advance(5 * 60)

		 for i, node := range nodes {
			if node.NetworkID == 3 {
		 	  logProgress(i, node.Server().PeerCount(),node.Server().ProtocolManager().PeerCount())
			}
		 }

		 logProgress("------")
	 }

	//godes.Advance(5 * 60)
	interval := 0.1
	times := 0
	counter := 0
	for times > 0 {
		times-=1

		godes.Advance(interval)
		//interval+=1
		key := core.BankKey

		txs := make(types.Transactions,0)

		//tx := types.NewTransaction(uint64(times), common.Address(acc1Addr), big.NewInt(1000), 10000, big.NewInt(1999), nil)
		tx, _ := types.SignTx(types.NewTransaction(uint64(counter), common.Address(acc2Addr), big.NewInt(1), 1000000, big.NewInt(10000000000), nil), types.HomesteadSigner{}, key)
		txs = append(txs, tx)
		nodeIndex := rand.Intn(config.SimConfig.NodeCount)
		util.Print("Adding new tx to node", nodeIndex + 1, "nonce", tx.Nonce())

		errors := nodes[nodeIndex].Server().GetProtocolManager().AddTxs(txs)
		for _, err := range errors {
			if err != nil {
				util.Print("Error when adding new tx nonce", tx.Nonce())
			}
		}

		for i := 0; i < config.SimConfig.NodeCount; i+=1 {
//			logProgress(i, nodes[i].Server().GetProtocolManager().PendingTxCount())
		}
		counter += 1
	}

	times = 10
	for times > 0{
		times -= 1
		godes.Advance(10)
		for i := 0; i < config.SimConfig.NodeCount; i+=1 {
	//		logProgress(i, nodes[i].Server().GetProtocolManager().PendingTxCount())
		}

	}

	*/


/*
	var testAccount, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")

	const txsyncPackSize = 100 * 1024

	// Fill the pool with big transactions.
	const txsize = txsyncPackSize / 10
	alltxs := make([]*types.Transaction, 100)
	for nonce := range alltxs {
		alltxs[nonce] = newTestTransaction(testAccount, uint64(nonce), txsize, int64(nonce+1))
	}

	nodes[rand.Intn(config.SimConfig.NodeCount)].Server().GetProtocolManager().AddTxs(alltxs)

*/

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



func waitForEnd(nodes []*network.Node)  {

	progressSimToEnd()

	if godes.GetSystemTime() > config.SimConfig.SimulationTime {
		config.SimConfig.SimulationTime = godes.GetSystemTime()
	}

	config.LogConfig.Logging = true

	util.Log("Simulation end after:", sysTime.Since(startTime))

	godes.Clear()

	showStats(nodes)


	es := nodes[0].Server().(*eth.Ethereum)
	bc := es.BlockChain()

	max := bc.CurrentBlock().NumberU64()

	util.Print("Last block", max)

	for i := 0; i < config.SimConfig.NodeCount ; i+=1  {
		ob := nodes[i].Server().(*eth.Ethereum).BlockChain().CurrentBlock()
		util.Print(i, ob.NumberU64())
	}
	size := common.StorageSize(0)
	total := 0
	for i := max;i >= 1 ; i-=1  {
		block := bc.GetBlockByNumber(uint64(i))
		if block != nil {
			signAddr, err := bc.Engine().Author(block.Header())
			if err != nil {
				util.LogError(err)
			}
			signer := findNodeByAddress(nodes, signAddr)

			util.Print(block.Number(), "tx count", block.Transactions().Len(), "signer", signer)
			size += block.Size()
			size += block.Header().Size()

			total += len(block.Transactions())
		} else {
			util.Print("block", i, "je nil")
		}
	}

	util.Print("Total num of txs", total)
	util.Print("BC size", size)
}

func findNodeByAddress(nodes []*network.Node, address common.Address) *network.Node {
	for _, node := range nodes {
		if node.Address() == address {
			return node
		}
	}
	return nil
}

func showStats(nodes []*network.Node)  {
	export.Stats(nodes)
}


func main()  {
	runSim()
	//export.Test()
}
