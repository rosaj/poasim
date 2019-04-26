package config

import (
	"github.com/agoussia/godes"
)

var repetition = false

var TrDistr = godes.NewExpDistr(repetition)

var NodePingDistr = godes.NewExpDistr(repetition)

type config struct {
	SimulationTime float64

	ValidatorCount int

	TrArrivalInterval float64

	NodePingTime float64

	BlockTime float64
}


var test = config{
	SimulationTime:  1 * 60,
	ValidatorCount: 6,
	NodePingTime: 0.25,
	TrArrivalInterval: 0.5,
	BlockTime: 15,
}

var SimConfig = test




func(config *config) NextTrInterval() (interval float64){
	interval = TrDistr.Get(1 / config.TrArrivalInterval)
//	Log("TrInterval: ", interval)
	return
}

func (config *config) NextNetworkLatency() (interval float64) {
	interval = NodePingDistr.Get(1 / config.NodePingTime)
	return
}

