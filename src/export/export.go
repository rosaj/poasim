package export

import (
	"../common"
	"../config"
	"fmt"
	"log"

	"../generate"
	"../metrics"
	. "../network"
	"../util"
	"encoding/csv"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
)

type metricFn func(value map[float64]float64, count map[float64]int) map[float64]float64

var avgFn = func(value map[float64]float64, count map[float64]int) map[float64]float64 {
	vals := make(map[float64]float64, len(value))

	for v, val := range value {
		vals[v] = val / float64(count[v])
	}

	return vals
}


var sumFn = func(value map[float64]float64, count map[float64]int) map[float64]float64 {
	return value
}

var metricFnMapper = map[config.DataCollectType] metricFn {
	config.Sum: 	sumFn,
	config.Average: avgFn,
}


func addPoints(vals map[float64]float64, key string, ptss map[string]plotter.XYs)  {

	// To store the keys in slice in sorted order
	var keys []float64
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Float64s(keys)


	pts := make(plotter.XYs, len(vals))


	for y, key := range keys {

		pts[y].X = key
		pts[y].Y = vals[key]
		//fmt.Println(key, value)
	}
	ptss[key] = pts
}


func Stats(nodes []*Node)  {


	//stats := [...]string{ OnlineNodes}
	stats := config.MetricConfig.Metrics

	all := make(map[string]map[float64]float64, 0)
	count := make(map[string]map[float64]int, 0)

	for _, stat := range stats {

		all[stat] = make(map[float64]float64)
		count[stat] = make(map[float64]int)

		for _, node := range nodes {

			col := node.IMetricCollector.(*common.MetricCollector)
			data := col.Collect(stat)

			for key, val := range data {

				all[stat][key] += val
				count[stat][key] += 1

			}
		}

	}

	/*
	for k, v := range all {
		t:=0.0
		for _, v1 := range v {
			t+=v1
		}
		util.Print(k, t)
	}
	*/


	ptss := make(map[string]plotter.XYs)

	mc := config.MetricConfig

	for key, value := range all {
		colType := mc.CollectType

		if mc.MetricCollectType != nil {
			if val, ok := mc.MetricCollectType[key]; ok {
				colType = val
			}
		}

		addPoints(metricFnMapper[colType](value, count[key]), key, ptss)

		/*

		if config.MetricConfig.CollectType == config.Average {

			vals := make(map[float64]float64, len(value))

			for v, val := range value {
				vals[v] = val / float64(count[key][v])
			}

			addPoints(vals, key, ptss)

		} else if config.MetricConfig.CollectType == config.Sum {

			addPoints(value, key, ptss)
		}

		 */
	}

	// mora bit na kraju, jer se inace pregazi praznim vrijednostima
	if all[OnlineNodes] != nil {
		nStats := GetNodeStats()
		addPoints(nStats, OnlineNodes, ptss)
	}

	if all[metrics.TxsArrival] != nil {
		addPoints(generate.GetTxsStats(), metrics.TxsArrival, ptss)
	}



	util.Print("All msg gathered")

	lines := make([]interface{}, 0)

	for _, name := range stats {
		lines = append(lines, name, ptss[name])
	}

	p, err := plot.New()
	if err != nil {
		panic(err)
	}

	plotutil.AddLines(p, lines...)

	p.Title.Text = fmt.Sprintf("Messages, nodes: %d, node arrival every: %s, grouped every: %s",
		config.SimConfig.NodeCount,
		util.ToDuration(config.SimConfig.NextNodeArrival()).String(),
		util.ToDuration(config.MetricConfig.GroupFactor).String())

	p.X.Label.Text = "Time"
	p.Y.Label.Text = "Count"


	if err != nil {
		panic(err)
	}


	xNames := make([]string, int(config.SimConfig.SimulationTime/config.MetricConfig.GroupFactor))

	for i := range xNames {
		if math.Mod(float64(i), float64(len(xNames)/5)) == 0{
			xNames[i] = util.ToDuration(float64(i) * config.MetricConfig.GroupFactor).String()
		}
	}

	p.NominalX(xNames...)

	fmt.Println("Points prepared")


	exportType := config.MetricConfig.ExportType

	name := fmt.Sprintf("%s_%s_%s_%s.%s",
		config.SimConfig.SimMode,
		util.ToDuration(config.SimConfig.SimulationTime).String(),
		strconv.Itoa(config.SimConfig.NodeCount),
		strconv.Itoa(int(config.MetricConfig.GroupFactor)),
		exportType)

	if exportType == config.PNG {
		// Save the export to a PNG file.
		if err := p.Save(vg.Length(1280), vg.Length(768), name); err != nil {
			panic(err)
		}
	} else if exportType == config.CSV {
		csvExport(ptss, name)
	}


	fmt.Println("wrote to", name)


}

func csvExport(data map[string]plotter.XYs, name string) error {

	//name = "eth.csv"

	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	name = dir + "/src/res/" + name

	file, err := os.Create(name)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// To store the keys in slice in sorted order
	var keys []string
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)


	headers := make([]string, 0)
	headers = append(headers, "ROWID", "Time")

	for _, key := range keys {
		headers = append(headers, key)
	}

	if err := writer.Write(headers); err != nil {
		return err
	}
	groupFactor := config.MetricConfig.GroupFactor

	rows := make([][]string, int(config.SimConfig.SimulationTime/groupFactor)+1)

	for i := range rows {

		row := make([]string, 0)

		x := float64(i) * groupFactor

		row = append(row, strconv.Itoa(int(x)), util.ToDuration(x).String())

		rows[i] = row
	}

	for _, key := range keys {
		vals := data[key]

		ind := 0
		for i := range rows {

			x := float64(i) * groupFactor
			// ako do kraja treba popunit sa praznim mjestima ili
			// ako je izmedu x-eva praznega mjesta
			// onda pisi prazna mjesta
//			util.Print(i, ind, x, vals[ind].X )
			if  ind >= len(vals) || x < vals[ind].X * groupFactor {
				rows[i] = append(rows[i], "")
				continue
			}
			rows[i] = append(rows[i],  strconv.Itoa(int(vals[ind].Y)))
			ind++
		}

	}


	for _, row := range rows {

		if err := writer.Write(row); err != nil {
			return err
		}
	}

//	util.Print(data)
	return nil
}


func Test() {
	rand.Seed(int64(0))

	p, err := plot.New()
	if err != nil {
		panic(err)
	}

	p.Title.Text = "Plotutil example"
	p.X.Label.Text = "X"
	p.Y.Label.Text = "Y"

	err = plotutil.AddLines(p,
		"First", randomPoints(10000000))
	if err != nil {
		panic(err)
	}

	// Save the export to a PNG file.
	if err := p.Save(vg.Length(2880), vg.Length(1800), "points.png"); err != nil {
		panic(err)
	}

}

// randomPoints returns some random x, y points.
func randomPoints(n int) plotter.XYs {

	vals := make(map[float64]float64)

	for i := 0; i < n; i++ {
		val := config.SimConfig.NextNodeSessionTime()
		val = math.Round(val*100)/100

		vals[val] += 1
		//fmt.Println(val, vals[val])
	}


	// To store the keys in slice in sorted order
	var keys []float64
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Float64s(keys)


	pts := make(plotter.XYs, len(vals))

	y := 0

	for _, key := range keys {

		pts[y].X = key
		pts[y].Y = vals[key]
		y+=1
		//fmt.Println(key, value)
	}

	/*
	for i := range pts {
		if i == 0 {
			pts[i].X = rand.Float64()
		} else {
			pts[i].X = pts[i-1].X + rand.Float64()
		}
		pts[i].Y = pts[i].X + 10*rand.Float64()
	}
	*/

	return pts
}