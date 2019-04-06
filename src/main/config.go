package main

import "github.com/agoussia/godes"


var TrDistr = godes.NewExpDistr(true)

var NodePingDistr = godes.NewExpDistr(true)

type Config struct {
	SimulationTime float64

	ValidatorCount int

	NodeCount int

	TrArrivalInterval float64

	NodePingTime float64

	BlockTime float64
}


var Test = Config{
	SimulationTime:  1 * 60,
	ValidatorCount: 6,
	NodeCount: 0,
	NodePingTime: 0.05,
	TrArrivalInterval: 0.5,
	BlockTime: 15,
}



func(config *Config) nextTrInterval() (interval float64){
	interval = TrDistr.Get(1 / config.TrArrivalInterval)
	//interval = 10
//	Log("TrInterval: ", interval)
	return
}


func(config *Config) nextNodeInterval() (interval float64){
	interval = NodePingDistr.Get(1 / config.NodePingTime)
	//interval = 0.05
//	Log("NodeInterval: ", interval)
	return
}

