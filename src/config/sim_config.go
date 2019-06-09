package config

import (
	"../network/eth/params"
	"math/big"
	"time"
)

var SimConfig = config {

	SimulationTime: (1 * 10 * time.Minute).Seconds(),

	NodeCount: 6,

	NodeStabilisationTime:  1 * time.Minute.Seconds(),

	ChurnEnabled: false,

	NodeArrivalDistr: NewNormalDistr((15*time.Second.Seconds()), 0),

	NodeSessionTimeDistr: NewExpDistr(1 /( 1 * (time.Minute).Seconds())),

	NodeIntersessionTimeDistr: NewExpDistr( 1 / (1 * time.Minute).Seconds()),

	NodeLifetimeDistr: NewExpDistr(1 / (11111116 * time.Minute.Seconds())),

	NetworkLatency:  NewLogNormalDistr(.209,.157),// u metodi NextNetworkLatency dodano /10

	MaxPeers: 25,

	MinerCount: 6,

	BlockTime: 15,

	TransactionIntervalDistr: NewExpDistr(0.2),

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

var MetricConfig  = metricConfig {

	GroupFactor: 60,

}


var EthConfig = EthereumConfig {
	ChainConfig: ChainConfig,
	MinerConfig: DefaultMinerConfig,
	TxPoolConfig: DefaultTxPoolConfig,
}


var ChainConfig = &chainConfig {
	Clique: & CliqueConfig {
		Period: 15,
		Epoch:	30000,
	},
}

var	TxPoolConfig = &txPoolConfig {

	PriceLimit: 1,
	PriceBump:  10,

	AccountSlots: 1000000000,
	GlobalSlots:  1000000000,
	AccountQueue: 1000000000,
	GlobalQueue:  1000000000,

	Lifetime: 3 * time.Hour,
}

var DefaultTxPoolConfig = &txPoolConfig {
	PriceLimit: 1,
	PriceBump:  10,

	AccountSlots: 16,
	GlobalSlots:  4096,
	AccountQueue: 64,
	GlobalQueue:  1024,

	Lifetime: 3 * time.Hour,
}


var	MinerConfig = &minerConfig {
	GasFloor: 999999999999,
	GasCeil:  999999999999,
	GasPrice: big.NewInt(params.Wei),
	Recommit: 3 * time.Second,
}

var	DefaultMinerConfig = &minerConfig {
	//Default 8000000
	GasFloor: 8000000,
	GasCeil:  8000000,
	GasPrice: big.NewInt(params.Wei),
	Recommit: 3 * time.Second,
}