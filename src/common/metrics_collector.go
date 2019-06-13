package common

import (
	. "../config"
)

/*
type CalcFn  func(sum float64, count float64) float64

var AvgFn = func(sum float64, count float64) float64 {
	return sum / count
}

var SumFn = func(sum float64, count float64) float64 {
	return sum
}

var statFns = make(map[string]func(sum float64, count float64) float64, 0)

func RegisterStatFn(name string, fn CalcFn) string {
	statFns[name] = fn
	return ""
}

func statFn(name string)  CalcFn {
	fn := statFns[name]
	if fn != nil {
		return fn
	}
	return SumFn
}

*/

type MetricCollector struct {
	metrics	map[string]map[float64][]int
}

func NewMetricCollector() *MetricCollector {
	return newMetricCollector()
}
func newMetricCollector() *MetricCollector {
	return &MetricCollector{
		metrics: make(map[string]map[float64][]int, 0),
	}
}

func getTime() float64 {
	return MetricConfig.GetTimeGroup()
}
func (mc *MetricCollector) ensureMetric(name string, numOfElems int)  {
	if  mc.metrics[name] == nil {
		mc.metrics[name]= make(map[float64][]int)
	}
	t := getTime()
	if mc.metrics[name][t] == nil {
		mc.metrics[name][t] = make([]int, numOfElems)
	}

}
func (mc *MetricCollector) UpdateWithValue(name string, value int) {
	mc.ensureMetric(name, 1)
	mc.metrics[name][getTime()][0] += value
}

func (mc *MetricCollector) Update(name string)  {
	mc.UpdateWithValue(name, 1)
}

func (mc *MetricCollector) Set(name string, value int)  {
	mc.ensureMetric(name, 0)
	mc.metrics[name][getTime()] = append(mc.metrics[name][getTime()], value)
}

func (mc *MetricCollector) GetMetrics() map[string]map[float64][]int	{
	return mc.metrics
}

func (mc *MetricCollector) CollectMetrics() map[string]map[float64]float64  {
	stats := make(map[string]map[float64]float64)

	for key := range mc.metrics {
		stats[key] = mc.Collect(key)
	}

	return stats
}

func (mc *MetricCollector) Collect(name string) map[float64]float64 {
	stats := make(map[float64]float64, 0)


	for time, count := range mc.metrics[name] {
		sum := 0

		for _, val := range count {
			sum += val
		}

		stats[time] = float64(sum)/float64(len(count))
		//stats[time] = statFn(name)(float64(sum), float64(len(count)))
	}



	return stats
}