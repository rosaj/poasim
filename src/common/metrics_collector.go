package common

import (
	. "../config"
)

var GlobalCollector = newMetricCollector()

type MetricCollector struct {
	metrics	map[string]map[float64][]int
}

func NewMetricCollector() *MetricCollector {
	return GlobalCollector
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
func (mc *MetricCollector) Update(name string, value int) {
	mc.ensureMetric(name, 1)
	mc.metrics[name][getTime()][0] += value
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

	for key, val := range mc.metrics {
		stats[key] = make(map[float64]float64, 0)

		for k, v := range val {
			sum := 0

			for _, v1 := range v {
				sum += v1
			}

			stats[key][k] = float64(sum)// / float64(len(v))
		}
	}

	return stats
}