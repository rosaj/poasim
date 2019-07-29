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

//	logProgress("Starting minting")
//	startMinting(nodes)

//	godes.Advance(config.SimConfig.NodeStabilisationTime)

	if config.SimConfig.SimMode == config.BLOCKCHAIN {
		logProgress("Tx arrival")
		generate.AsyncTxsDistr(nodes[:config.SimConfig.NodeCount])
	}

	st := godes.GetSystemTime()

	p := float64(config.ChainConfig.Clique.Period)

	f := st / p
	f += 3

	godes.Advance(float64(int(f)) * p - st)
	//godes.Advance(float64(config.ChainConfig.Clique.Period*2))

	startMinting(nodes)

	//godes.Advance(config.SimConfig.SimulationTime - godes.GetSystemTime() - 10)
	//printDEVp2pConnections(nodes)
//	ScenarioNodeLeavingNetwork(nodes[:len(nodes) - bootNodeCount],  config.SimConfig.NodeCount/2, sysTime.Hour)

	waitForEnd(nodes)

	return calcFinality()
}

func startMinting(nodes []*network.Node)  {
	logProgress("Starting minting")

	for _, node := range nodes {
		if srv := node.Server(); srv != nil {
			e := srv.(*eth.Ethereum)
			e.StartMining()
			//ebase, _ := e.Etherbase()
			//e.Miner().Start(ebase)
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

	util.Print(w/float64(count), f/float64(count))

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


// N = 6, 30 min tx gen

// Clique
// [10.644584650405621 0.7702570412267236] [18.141370405959435 1.5131401629587136]
// [19.049973887238487 1.8954115167774632] [19.527715781125263 1.1191790523983882]
// [10.450126067277703 0.805763699414532]  [10.085720254228253 0.46691036990985746]
// [11.540757960641251 0.9326180247336314] [12.53245201313582 0.5088286611291667]
// [10.594739840346156 0.7204162716169101] [19.97866988049985 1.7643623774337656]
// [16.809031954535886 1.593253034731311]  [9.777513624618127 0.39285235240670396]

// [10.563771199144261 0.8587982293488683] [9.84187098302284 0.3278707975471355]
//											[9.834793401475922 0.4251617664794296]
//											[9.860336464607737 0.3667282087741384]
// [22.637859543877667 1.7693359863163023]



// [14.160384158657982 0.7318052271237669]
// [14.35015646644988 1.1294275148468413]
// [14.685803037396374 0.6859335250849962]
// [14.675537194728602 0.4880868507621693]
// [13.916372714692553 0.6943042814524195]



// Test 2

// Clique
// [31.821962178308606 3.183370285944578] [29.02281041122921 3.813252343940973]
// [32.12229284731633  3.098237327952183] [27.92075315539283 3.9495823643828922]
// [30.0207672270547   3.001125880937945] [29.273140392083697 2.0522627160059295]

// [31.530323435569745 3.780846358106992] [32.82707703096588 2.9811086789022574]
// [31.542300310906608 4.242131730177002] [32.800666372509035 2.5695785593476312]
// [32.51803822113169  5.512339799623903]
// [31.82563629147404  5.769178076185182]

// [33.9395999658245 4.470757975939147]
// [38.11552512248735 4.152785268060193]
// [30.75365538338261 2.3657264531257476]


// Aura
// [18.88447662096721 0.4036231348647846]
// [14.1460955698441 0.38272917115313865]
// [43.276768318785 0.3938832437835324]
// [15.778308505525727 0.3937070715876581]
// [14.078821327171982 0.393113799027147]
// [36.11013440533574 0.2390920363243668]
// [56.78411346943759 0.22957461821505898]
// [30.48286856600543 0.2350043526483788]


// [101.65701315758427 0.24307169830126596]
// [44.929448502676905 0.2320711892209375]
// [33.64603621060383 0.23834797940609628]
// [33.1590702704571 0.23364316155657675]
// [37.50366629978566 0.2330175834908822]

// N = 60

// AURA
// [42.505898737852846 1.1711130366342775]

// Session time = 30 min
// [75.18428747639746 2.0166938134405665]

// CLIQUE
//



// Clique
// [29.51844876279442 2.341196276718656]
// [31.741668020318354 3.3021119142864888] 233
// [31.768264694816526 3.7178341068894163] 233
// [37.64404998722515 3.912207250777633]   233
// [34.65384539537706 2.4515492607671883]  233
// [32.03777731248077 3.482937275510884]   233

// Aura
// [59.04868361498625 0.22614765302581533]
// [50.83563231251301 0.22994451592354165] 173
// [24.49952139879624 0.2362228748842413]  162
// [77.73816114755246 0.2181281727131488]
// [44.55385583485741 0.22739451281810502]
// [64.962311645561 0.23136476875271328]
// [32.52882368838659 0.23747463701458577]

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

	showStats(nodes)
	//showStats(nodes[0:len(nodes) - bootNodeCount ])

}

func showStats(nodes []*network.Node)  {
	export.Stats(nodes)

	if config.SimConfig.SimMode == config.BLOCKCHAIN {
		PrintBlockchainStats(nodes, true)
	}
}


func main() {
	//RunSim(71, 20, 19, 1)
	StartSim()
	//RunSim(5, 15)
}


//go build -buildmode=c-shared -o poasim.so  src/main/main.go