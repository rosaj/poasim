package config

import (
	"github.com/agoussia/godes"
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
	NodeArrivalDistr distribution

	// distribucija trajanja sesija cvorova
	NodeSessionTimeDistr distribution

	// distribucija trajanja nedostupnosti(offline) cvorova
	NodeIntersessionTimeDistr distribution

	// vrijeme od kada se cvor spoji na mrezu do kada se zauvjek odspoji
	NodeLifetimeDistr distribution

	// latencija slanja poruke preko mreze
	NetworkLatency distribution

	//NetworkUnreliability float64
	//TODO: impl gubljenje paketa ili u obliku distr ili postotka
	//LostMessagesDistr distribution

	MinerCount int

	BlockTime float64

	TransactionIntervalDistr distribution
}

type logConfig struct {
	// flag da li je globalno logiranje ukljuceno
	Logging bool

	// da li logirat poruke vezane uz Message tip
	LogMessages bool
}

type metricConfig struct {
	// jedinica po kojoj se grupiraju poruke
	// npr. 60 znaci da se poruke grupiraju po minuti
	MsgGroupFactor float64

}


var SimConfig = config {

	SimulationTime: (3 * time.Hour).Seconds(),

	NodeCount: 10000,

	NodeStabilisationTime:  30 * time.Minute.Seconds(),

	ChurnEnabled: true,

	NodeArrivalDistr: NewNormalDistr((80*time.Minute.Seconds())/10000, 0),

	NodeSessionTimeDistr: NewExpDistr(1 /( 1 * (time.Hour).Seconds())),

	NodeIntersessionTimeDistr: NewExpDistr( 1 / (1 * time.Minute).Seconds()),

	NodeLifetimeDistr: NewExpDistr(1 / (111115 * time.Hour.Seconds())),

	NetworkLatency:  NewLogNormalDistr(.209,.157),// u metodi NextNetworkLatency dodano /10

	MinerCount: 6,

	BlockTime: 15,

	TransactionIntervalDistr: NewExpDistr(0.2),

}

var LogConfig = logConfig {

	Logging: false,

	LogMessages: false,
}

var MetricConfig  = metricConfig {

	MsgGroupFactor: 60,


}

func (config *config) NextTrInterval() (interval float64) {
	interval = config.TransactionIntervalDistr.nextValue()
	//	Log("TrInterval: ", interval)
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
	//fmt.Println(interval)
	return
}

func (config *config) NextNodeIntersessionTime() (interval float64)  {
	interval = clampToSimTime(config, config.NodeIntersessionTimeDistr)
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





func clampToSimTime(config *config, distr distribution) (interval float64)  {
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