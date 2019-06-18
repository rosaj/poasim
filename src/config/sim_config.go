package config

import (
	"../metrics"
	"../network/eth/params"
	"math/big"
	"time"
)

var SimConfig = config {

	SimulationTime: (1 * 6 * time.Hour).Seconds(),

	NodeCount: 12,

	NodeStabilisationTime:  1 * time.Minute.Seconds(),

	ChurnEnabled: true,

	NodeArrivalDistr: NewNormalDistr((15*time.Second.Seconds()), 0),

	NodeSessionTimeDistr: NewExpDistr(1 /( 24 * (time.Hour).Seconds())),

	NodeIntersessionTimeDistr: NewExpDistr( 1 / (30 * time.Second).Seconds()),

	NodeLifetimeDistr: NewExpDistr(1 / (11111116 * time.Minute.Seconds())),

	NetworkLatency:  NewLogNormalDistr(.209,.157),// u metodi NextNetworkLatency dodano /10

	MaxPeers: 25,

	MinerCount: 6,

	TransactionIntervalDistr: NewExpDistr(1/0.8),

	SimMode: ETHEREUM,

	ActorCount:  1000,

}

var LogConfig = logConfig {

	Logging: false,

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

var MetricConfig  = metricConfig {

	GroupFactor: 60,

	ExportType: PNG,

//	Metrics: append (make([]string, 0), metrics.GasLimit),
	////	Metrics: metrics.TxPoolMetrics[:],
	Metrics: metrics.AllTxPoolMetrics[:],
}


var EthConfig = EthereumConfig {
	ChainConfig: ChainConfig,
	MinerConfig: testMinerConfig,
	TxPoolConfig: defaultTxPoolConfig,
}


var ChainConfig = &chainConfig {
	Engine:	CLIQUE,
	Clique: & CliqueConfig {
		Period: 15,
		Epoch:	30000,
	},
	Aura:	&AuraConfig {
		Period: 15,
		Epoch:  30000,
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
	//Default 8000000
	GasFloor: 8000000,
	GasCeil:  8000000,
	GasPrice: big.NewInt(params.Wei),
	Recommit: 3 * time.Second,
}