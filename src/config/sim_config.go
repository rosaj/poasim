package config

import (
	"../metrics"
	"../network/eth/params"
	"math/big"
	"time"
)

var SimConfig = config {

	SimulationTime: (1 * 6 * time.Hour).Seconds(),

	NodeCount: 100,

	NodeStabilisationTime:  10 * time.Minute.Seconds(),

	ChurnEnabled: true,

	NodeArrivalDistr: NewNormalDistr((80*time.Minute.Seconds())/100, (80*time.Minute.Seconds())/1000),

	NodeSessionTimeDistr: NewExpDistr(1 /( 1 * (time.Hour).Seconds())),

	NodeIntersessionTimeDistr: NewExpDistr( 1 / (1 * time.Minute).Seconds()),

	NodeLifetimeDistr: NewExpDistr(1 / (6222222 * time.Hour.Seconds())),

	NetworkLatency:  NewLogNormalDistr(.209,.157),// u metodi NextNetworkLatency dodano /10

	MaxPeers: 25,

	MinerCount: 6,

	SimMode: DISCOVERY,

	FastMode: true, //TODO: this

	TxGeneratorConfig: txGeneratorConfig{

		ActorCount: 10000,

		TransactionIntervalDistr: NewExpDistr(1/0.05),

		TxPriceDistr: NewNormalDistr(3, 1),

		Duration: 30 * time.Minute,

	},
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

	CollectType: Sum,

//	Metrics: append (make([]string, 0), metrics.GasLimit),
	////	Metrics: metrics.TxPoolMetrics[:],
//	Metrics: append(metrics.AllTxPoolMetrics[:], metrics.GasLimit),
	Metrics: metrics.DiscoveryMetrics[:],
//	Metrics: metrics.DiscoveryMetrics[:],
//	Metrics: metrics.AllTxPoolMetrics[:],
}


var EthConfig = EthereumConfig {
	ChainConfig: ChainConfig,
	MinerConfig: defaultMinerConfig,
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
	GasFloor: 8000000,
	GasCeil:  8000000,
	GasPrice: big.NewInt(params.Wei),
	Recommit: 3 * time.Second,
}