package common

import (
	. "../config"
	"github.com/agoussia/godes"
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



var GlobalMetricCollector = NewMetricCollector()


type MetricCollector struct {
	metrics	map[string]map[float64]*entry
}

func NewMetricCollector() *MetricCollector {
	return newMetricCollector()
}

type entry struct {
	sum int
	count int
}

func newMetricCollector() *MetricCollector {
	return &MetricCollector{
		metrics: make(map[string]map[float64]*entry, 0),
	}
}

func getTime() float64 {
	return MetricConfig.GetTimeGroup()
}
func (mc *MetricCollector) ensureMetric(name string, numOfElems int)  {
	t := getTime()

	if  mc.metrics[name] == nil {
		mc.metrics[name]= make(map[float64]*entry)
	}
	if mc.metrics[name][t] == nil {
		 mc.metrics[name][t] = &entry{0, numOfElems}
	}

}
func (mc *MetricCollector) UpdateWithValue(name string, value int) {
	mc.ensureMetric(name, 1)
	mc.metrics[name][getTime()].sum += value
}

func (mc *MetricCollector) Update(name string)  {
	mc.UpdateWithValue(name, 1)
}

func (mc *MetricCollector) Set(name string, value int)  {
	mc.ensureMetric(name, 0)
	t := getTime()
	mc.metrics[name][t].sum += value
	mc.metrics[name][t].count++

}



func (mc *MetricCollector) Collect(name string) map[float64]float64 {
	stats := make(map[float64]float64, 0)

	for k, v := range mc.metrics[name] {
		stats[k] = float64(v.sum)/float64(v.count)
	}

	return stats
}


func (mc *MetricCollector) Get(name string) map[float64]int {
	stats := make(map[float64]int, 0)

	for k, v := range mc.metrics[name] {
		stats[k] = v.sum
	}
	return stats
}



var FinalityMetricCollector = newTxMetricCollector()


type txEntry struct {
	Submitted 	float64
	Included	[]float64
	Inserted	[]float64
	Forked		[]float64
}

func (tx *txEntry) CalcFinality() (float64, float64) {
	last := 0.0

	if insLen := len(tx.Inserted); insLen > 0 {
		last = tx.Inserted[insLen - 1]
	}

	if forkLen := len(tx.Forked); forkLen > 0  && tx.Forked[forkLen - 1] > last{
		last = tx.Forked[forkLen - 1]
	}


	return last - tx.Submitted, last - tx.Included[0]
}

type TxMetricCollector struct {
	metrics	map[string]*txEntry
	sync 	bool
}

func newTxMetricCollector() *TxMetricCollector {
	return &TxMetricCollector{
		metrics: make(map[string]*txEntry, 0),
		sync: 	 false,
	}
}


func (mc *TxMetricCollector) TxSubmitted(name string)  {

	tx := &txEntry{ Submitted: godes.GetSystemTime(), 
					Included:  make([]float64, 0),
				    Inserted:  make([]float64, 0),
				    Forked:    make([]float64, 0),}
	

	mc.metrics[name] = tx
}


func (mc *TxMetricCollector) TxIncluded(name string)  {
	mc.metrics[name].Included = append(mc.metrics[name].Included, godes.GetSystemTime())
}

func (mc *TxMetricCollector) TxInserted(name string)  {
	if mc.sync {
		return
	}
	mc.metrics[name].Inserted = append(mc.metrics[name].Inserted, godes.GetSystemTime())
}

func (mc *TxMetricCollector) TxForked(name string)  {
	mc.metrics[name].Forked = append(mc.metrics[name].Forked, godes.GetSystemTime())
}

func (mc *TxMetricCollector) Collect() map[string]*txEntry {
	return mc.metrics
}

func (mc *TxMetricCollector) SyncingBlocksStart()  {
	mc.sync = true
}

func (mc *TxMetricCollector) SyncingBlocksEnd()  {
	mc.sync = false
}

/*
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

func (mc *MetricCollector) Get(name string) map[float64][]int {
	return mc.metrics[name]
}
*/