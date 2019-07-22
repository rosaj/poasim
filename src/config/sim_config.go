package config

import (
	"../metrics"
	"../network/eth/params"
	"math/big"
	"time"
)

var SimConfig = config {

	SimulationTime: (1 * 60 * time.Minute).Seconds(),

	NodeCount: 60,

	NodeStabilisationTime:  5 * time.Minute.Seconds(),

	ChurnEnabled: false,

	NodeArrivalDistr: NewNormalDistr((13*time.Second.Seconds()), 0),

	NodeSessionTimeDistr: NewExpDistr(1 /( 1 * (time.Hour).Seconds())),

	NodeIntersessionTimeDistr: NewExpDistr( 1 / (1 * time.Minute).Seconds()),

	NodeLifetimeDistr: NewExpDistr(1 / (6111111 * time.Hour.Seconds())),

	NetworkLatency:  NewLogNormalDistr(.209,.157),// u metodi NextNetworkLatency dodano /10

	MaxPeers: 25,

	SimMode: BLOCKCHAIN,

	TxGeneratorConfig: txGeneratorConfig {

		ActorCount: 1000,

		//TransactionIntervalDistr: NewExpDistr(1/0.06),
		TransactionIntervalDistr: NewNormalDistr(0.04, 0.01),

		TxPriceDistr: NewNormalDistr(3, 1),

		Duration: 0 * time.Minute,

	},
}


var MetricConfig  = metricConfig {

	GroupFactor: 15,

	ExportType: PNG,

	CollectType: Average,

	Metrics: []string {
		metrics.InsertForkedBlock,
		metrics.ChainSplitDepth,
		metrics.SyncDiff,
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
	MinerConfig: defaultMinerConfig,
	TxPoolConfig: defaultTxPoolConfig,
}


var ChainConfig = &chainConfig {
	Engine:	CLIQUE,
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
	GasFloor: 999999999999,
	GasCeil:  999999999999,
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

	Logging: true,

	LogMessages: false,

	LogDialing: false,

	LogNode: false,

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