package config

import (
	"github.com/agoussia/godes"
	"math"
	"math/big"
	"time"
)

type config struct {

	// vrijeme trajanja simulacije u sekundama, ignorira se ako spajanje svih cvorova traje vise od simulacije
	SimulationTime float64

	// broj cvorova koji se trebaju dodati u simulaciju
	NodeCount int

	// ako je postavljeno na true cvorovi mogu napustat i dolazit natrag na mrezu
	ChurnEnabled bool

	// vrijme cekanja stabilizacije mreze nakon spajanja svih cvorova (nema odspajanja i spajanja cvorova)
	NodeStabilisationTime float64

	// distribucija dolaska cvorova na mrezu
	NodeArrivalDistr Distribution

	// distribucija trajanja sesija cvorova
	NodeSessionTimeDistr Distribution

	// distribucija trajanja nedostupnosti(offline) cvorova
	NodeIntersessionTimeDistr Distribution

	// vrijeme od kada se cvor spoji na mrezu do kada se zauvjek odspoji
	NodeLifetimeDistr Distribution

	// distribucija spajanja novih nodova nakon sto se inicijalno svi nodovi spoje i mreza se stablizira
	NewNodeArrivalDistr Distribution

	// latencija slanja poruke preko mreze
	NetworkLatency Distribution

	//NetworkUnreliability float64
	//TODO: impl gubljenje paketa ili u obliku distr ili postotka
	//LostMessagesDistr Distribution

	MaxPeers int

	DialRatio int

	SimMode mode

	FastMode bool


	TxGeneratorConfig txGeneratorConfig
}

type mode string

var (
	DISCOVERY 	= mode("rlpx")
	DEVp2p		= mode("devp2p")
	BLOCKCHAIN	= mode("bc")
)


type logConfig struct {
	// flag da li je globalno logiranje ukljuceno
	Logging bool

	// da li logirat poruke vezane uz Message tip
	LogMessages bool

	LogDialing bool

	LogNode	bool

	LogPeer	bool

	LogDiscovery bool

	LogServer bool

	LogEthServer bool

	LogWorker bool

	LogConsensus bool

	LogProtocol bool

	LogBlockchain bool

	LogTxPool bool

	LogDownload bool

	LogDatabase bool
}

type ExportType string
var (
	PNG = ExportType("png")
	CSV = ExportType("csv")
	NA  = ExportType("none")
)

type DataCollectType int
var	(
	Average		= DataCollectType(0)
	Sum			= DataCollectType(1)
)

type metricConfig struct {
	// jedinica po kojoj se grupiraju poruke
	// npr. 60 znaci da se poruke grupiraju po minuti
	GroupFactor float64

	Metrics []string

	ExportType ExportType

	CollectType DataCollectType

	MetricCollectType map[string]DataCollectType
	

}

type txGeneratorConfig struct {

	ActorCount int

	Duration time.Duration

	TransactionIntervalDistr Distribution

	TxPriceDistr Distribution

}


type EthereumConfig struct {
	ChainConfig *chainConfig
	// Mining options
	MinerConfig *minerConfig

	// Transaction pool options
	TxPoolConfig *txPoolConfig
}

type chainConfig struct {
	Engine ConsensusEngine
	Clique *CliqueConfig
	Aura   *AuraConfig
}

// CliqueConfig is the consensus engine configs for proof-of-authority based sealing.
type CliqueConfig struct {
	Period uint64 // Number of seconds between blocks to enforce
	Epoch  uint64 // Epoch length to reset votes and checkpoint
}


// AuraConfig is the consensus engine configs for proof-of-authority based sealing.
type AuraConfig struct {
	Period uint64 // Number of seconds between blocks to enforce
}

type ConsensusEngine int
var (
	CLIQUE	  = ConsensusEngine(1)
	AURA	  = ConsensusEngine(2)
)


type txPoolConfig struct {
	PriceLimit uint64 // Minimum gas price to enforce for acceptance into the pool
	PriceBump  uint64 // Minimum price bump percentage to replace an already existing transaction (nonce)

	AccountSlots uint64 // Number of executable transaction slots guaranteed per account
	GlobalSlots  uint64 // Maximum number of executable transaction slots for all accounts
	AccountQueue uint64 // Maximum number of non-executable transaction slots permitted per account
	GlobalQueue  uint64 // Maximum number of non-executable transaction slots for all accounts

	Lifetime time.Duration // Maximum amount of time non-executable transaction are queued
}


type minerConfig struct {
	GasFloor  uint64         // Target gas floor for mined blocks.
	GasCeil   uint64         // Target gas ceiling for mined blocks.
	GasPrice  *big.Int       // Minimum gas price for mining a transaction
	Recommit  time.Duration  // The time interval for miner to re-create mining work.
}


func (txGeneratorConfig *txGeneratorConfig) NextTrInterval() (interval float64) {
	interval = txGeneratorConfig.TransactionIntervalDistr.nextValue()
	//log.Print("TrInterval: ", interval)
	return
}

func (txGeneratorConfig *txGeneratorConfig) NextTxPrice() (price float64) {
	price = math.Max(txGeneratorConfig.TxPriceDistr.nextValue(), 1	)
	//log.Print("TxPrice: ", price)
	return
}


func (config *config) NextNetworkLatency() (interval float64) {
	interval = config.NetworkLatency.nextValue() / 10
	return
}

func (config *config) NextNodeArrival() (interval float64) {
	interval = config.NodeArrivalDistr.nextValue()
	//fmt.Println("NodeInterval: ", interval)
	return
}

func (config *config) NextNodeSessionTime() (interval float64) {
	interval = clampToSimTime(config, config.NodeSessionTimeDistr)
	//fmt.Println("Session time", time.Duration(interval) * time.Second)
	return
}

func (config *config) NextNodeIntersessionTime() (interval float64)  {
	interval = clampToSimTime(config, config.NodeIntersessionTimeDistr)
	//fmt.Println("Intersession time", interval)
	return
}

func (config *config) NextNodeLifetime() (interval float64) {
	interval = config.NodeLifetimeDistr.nextValue()
	//fmt.Println(interval)
	return
}

func (config *config) SimulationEnded() bool {
	return godes.GetSystemTime() >= config.SimulationTime
}





func clampToSimTime(config *config, distr Distribution) (interval float64)  {
	interval = distr.nextValue()

	sysTime := godes.GetSystemTime()

	if estimatedNodeArrival := config.NextNodeArrival() * float64(config.NodeCount); sysTime < estimatedNodeArrival{
		interval += estimatedNodeArrival + config.NodeStabilisationTime
		sysTime = estimatedNodeArrival

	} else if sysTime < config.NodeStabilisationTime {
		// session traje stabilization time + interval iz distribucije
		interval += config.NodeStabilisationTime - sysTime


		// da se ne prekoraci sim time
		if dif := config.SimulationTime - sysTime; dif >= 0 && interval > dif {
			interval = dif
		}
	}

	return
}

func (metricConfig *metricConfig) GetTimeGroup() float64 {
	return	math.Round(godes.GetSystemTime()/ metricConfig.GroupFactor)
}