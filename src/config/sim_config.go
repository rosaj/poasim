package config

import (
	"../metrics"
	"../network/eth/params"
	"math/big"
	"time"
)

var SimConfig = config{

	SimulationTime: (1 * 60 * time.Minute).Seconds(),

	NodeCount: 60,

	NodeStabilisationTime:  1 * time.Minute.Seconds(),

	ChurnEnabled: true,

	NodeArrivalDistr: NewNormalDistr((1.3*time.Second.Seconds()), 0),

	NodeSessionTimeDistr: NewExpDistr(1 /( 0.5 * (time.Hour).Seconds())),

	NodeIntersessionTimeDistr: NewExpDistr( 1 / (1 * time.Minute).Seconds()),

	NodeLifetimeDistr: NewExpDistr(1 / (6111111 * time.Hour.Seconds())),

	NetworkLatency:  NewLogNormalDistr(.209,.157),// u metodi NextNetworkLatency dodano /10

	MaxPeers: 25,

	DialRatio: 0,

	SimMode: BLOCKCHAIN,

	TxGeneratorConfig: txGeneratorConfig {

		ActorCount: 10000,

		//TransactionIntervalDistr: NewExpDistr(1/0.06),

		TransactionIntervalDistr: NewNormalDistr(calcTxArrival(), calcTxArrival()/5.0),

		//TransactionIntervalDistr: NewNormalDistr(0.4411764706, 0.1), // 34/15 sec

		//TransactionIntervalDistr: NewNormalDistr(0.04, 0.01), // 380/15 sec

		TxPriceDistr: NewNormalDistr(3, 1),

		Duration: 30 * time.Minute,

	},
}

func calcTxArrival() float64 {
	txGasCost := uint64(21000)


	p := float64(ChainConfig.Clique.Period)
	gas := EthConfig.MinerConfig.GasCeil - 84000//83000 //84000

	bNum := gas / txGasCost

	return p / float64(bNum)
}

var MetricConfig  = metricConfig {

	GroupFactor: 15,

	ExportType: PNG,

	CollectType: Average,

	Metrics: []string {
		metrics.InsertForkedBlock,
		metrics.ChainSplitDepth,
		metrics.SyncDiff,
		metrics.TxsPerBlock,
		metrics.TxsArrival,
		metrics.PendingTxs,
	},

//	Metrics: metrics.AllMetrics,
//	Metrics: []string{
//		metrics.MinedBlock,
//		metrics.TxsPerBlock,
//		metrics.TransactionUnderpriced,
//		metrics.NonceTooLow,
//		metrics.InsufficientFunds,
//	},

	MetricCollectType: map[string]DataCollectType{
		metrics.DiscoveryTable: 	Average,
		metrics.DEVp2pPeers:		Average,
		metrics.EthPeers:			Average,
		metrics.MinedBlock:			Sum,
		metrics.NEW_BLOCK_MSG:		Sum,
	},
}


var EthConfig = EthereumConfig {
	ChainConfig: ChainConfig,
	MinerConfig: testMinerConfig,
	TxPoolConfig: defaultTxPoolConfig,
}


var ChainConfig = &chainConfig {
	Engine:	AURA,
	Clique: &CliqueConfig {
		Period: 15,
		Epoch:	30000,
	},
	Aura:	&AuraConfig {
		Period: 15,
	},
}

var	testTxPoolConfig = &txPoolConfig {

	PriceLimit: 1,
	PriceBump:  10,

	AccountSlots: 1000000000,
	GlobalSlots:  1000000000,
	AccountQueue: 1000000000,
	GlobalQueue:  1000000000,

	Lifetime: 3 * time.Hour,
}

var defaultTxPoolConfig = &txPoolConfig {
	PriceLimit: 1,
	PriceBump:  10,

	AccountSlots: 16,
	GlobalSlots:  4096,
	AccountQueue: 64,
	GlobalQueue:  1024,

	Lifetime: 3 * time.Hour,
}


var	testMinerConfig = &minerConfig {
	GasFloor: 714000,
	GasCeil:  714000,
	GasPrice: big.NewInt(params.Wei),
	Recommit: 3 * time.Second,
}

var	defaultMinerConfig = &minerConfig {
	GasFloor: 8000000,
	GasCeil:  8000000,
	GasPrice: big.NewInt(params.Wei),
	Recommit: 3 * time.Second,
}


var LogConfig = logConfig {

	Logging: false,

	LogMessages: false,

	LogDialing: false,

	LogNode: true,

	LogPeer: false,

	LogDiscovery: false,

	LogServer: false,

	LogEthServer: false,

	LogWorker: false,

	LogConsensus: false,

	LogProtocol: false,

	LogBlockchain: false,

	LogTxPool: false,

	LogDownload: false,

	LogDatabase: false,
}