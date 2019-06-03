package main

import (
	. "../common"
	"../config"
	"../network"
	"../network/eth"
	"../network/eth/common"
	"../network/eth/core"
	"../network/eth/core/types"
	"../network/protocol"
	"../plot"
	"../util"
	"crypto/ecdsa"
	"fmt"
	"github.com/agoussia/godes"
	"github.com/ethereum/go-ethereum/crypto"
	"math"
	"math/big"
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

var (
	testBankKey, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	testBankAddress = crypto.PubkeyToAddress(testBankKey.PublicKey)
	testBankFunds   = big.NewInt(1000000000000000000)

	acc1Key, _ = crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
	acc2Key, _ = crypto.HexToECDSA("49a7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6fee")
	acc1Addr   = crypto.PubkeyToAddress(acc1Key.PublicKey)
	acc2Addr   = crypto.PubkeyToAddress(acc2Key.PublicKey)

	testContractCode         = common.Hex2Bytes("606060405260cc8060106000396000f360606040526000357c01000000000000000000000000000000000000000000000000000000009004806360cd2685146041578063c16431b914606b57603f565b005b6055600480803590602001909190505060a9565b6040518082815260200191505060405180910390f35b60886004808035906020019091908035906020019091905050608a565b005b80600060005083606481101560025790900160005b50819055505b5050565b6000600060005082606481101560025790900160005b5054905060c7565b91905056")
	testContractAddr         common.Address
	testContractCodeDeployed = testContractCode[16:]
	testContractDeployed     = uint64(2)

	testEventEmitterCode = common.Hex2Bytes("60606040523415600e57600080fd5b7f57050ab73f6b9ebdd9f76b8d4997793f48cf956e965ee070551b9ca0bb71584e60405160405180910390a160358060476000396000f3006060604052600080fd00a165627a7a723058203f727efcad8b5811f8cb1fc2620ce5e8c63570d697aef968172de296ea3994140029")
	testEventEmitterAddr common.Address

	testBufLimit = uint64(100)
)


// newTestTransaction create a new dummy transaction.
func newTestTransaction(from *ecdsa.PrivateKey, nonce uint64, datasize int, gasPrice int64) *types.Transaction {
	tx := types.NewTransaction(nonce, common.Address{}, big.NewInt(0), 100000, big.NewInt(gasPrice), make([]byte, datasize))
	tx, _ = types.SignTx(tx, types.HomesteadSigner{}, from)
	return tx
}

func runSim(){

	runtime.GOMAXPROCS(1)

	startTime = sysTime.Now()
	util.Log("start")

	nodes := runNodes()

	godes.Advance(config.SimConfig.NodeStabilisationTime)

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
	*/



	//godes.Advance(5 * 60)

	/*
	//interval := 0.1
	times := 300
	counter := 0
	for times > 0 {
		times-=1

		//godes.Advance(interval)
		//interval+=1
		key := acc1Key

		txs := make(types.Transactions,0)

		//tx := types.NewTransaction(uint64(times), common.Address(acc1Addr), big.NewInt(1000), 10000, big.NewInt(1999), nil)
		tx, _ := types.SignTx(types.NewTransaction(uint64(counter), common.Address(acc1Addr), big.NewInt(1), 1000000, big.NewInt(1), nil), types.HomesteadSigner{}, key)
		util.Print(counter, fmt.Sprintf("%x", tx.Hash()))
		txs = append(txs, tx)
		util.Print(rand.Intn(config.SimConfig.NodeCount))
		nodes[rand.Intn(config.SimConfig.NodeCount)].Server().GetProtocolManager().AddTxs(txs)
		counter += 1

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


	es := nodes[1].Server().(*eth.Ethereum)
	bc := es.BlockChain()

	max := bc.CurrentBlock().NumberU64()
	util.Print("Last block", max)

	for i := 1;i <= int(max) ; i+=1  {
		hash := bc.GetBlockByNumber(uint64(i))

		for y := 2; y < config.SimConfig.NodeCount; y+=1 {
			ohash := nodes[1].Server().(*eth.Ethereum).BlockChain().GetBlockByNumber(uint64(i))
			if hash != ohash {
				util.Print("Hash missmatch block", i, "node", y)
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
