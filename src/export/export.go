package export

import (
	"../common"
	"../config"
	. "../network"
	"../util"
	"encoding/csv"
	"fmt"

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

func addPoints(vals map[float64]float64, key string, ptss map[string]plotter.XYs)  {

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
	ptss[key] = pts
}
func Stats(nodes []*Node)  {


	//stats := [...]string{ eth.MinedBlock}
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

	ptss := make(map[string]plotter.XYs)

	if all[OnlineNodes] != nil {
		nStats := GetNodeStats()
		addPoints(nStats, OnlineNodes, ptss)
	}


	for key, value := range all {

		vals := make(map[float64]float64, len(value))

		for v, val := range value {
			vals[v] = val / float64(count[key][v])
		}
		addPoints(vals, key, ptss)

//		addPoints(value, key, ptss)

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

	name := fmt.Sprintf("msg_%s_%s_%s.%s",
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

	rows := make([][]string, 0)

	for _, v := range data["ALL"] {

		row := make([]string, 0)
		row = append(row, strconv.Itoa(int(v.X)),util.ToDuration(v.X * config.MetricConfig.GroupFactor).String())
		rows = append(rows, row)
	}

	all := data["ALL"]

	for _, key := range keys {
		value := data[key]
		i := 0
		for _, xy := range value {
			for all[i].X < xy.X {
				rows[i] = append(rows[i], "")
				i++
			}
			rows[i] = append(rows[i], strconv.Itoa(int(xy.Y)))
			i++
		}

	}

	for _, row := range rows {

		if err := writer.Write(row); err != nil {
			return err
		}
	}

/*
	for _, key := range keys {
		value := data[key]

		row := make([]string, 0)
		row = append(row, "Type")

		vals := make([]string, 0)
		vals = append(vals, key)

		for _, v := range value {
			row = append(row, util.ToDuration(v.X * config.MetricConfig.GroupFactor).String())
			vals = append(vals, strconv.Itoa(int(v.Y)))
		}

		if err := writer.Write(row); err != nil {
			return err
		}
		if err := writer.Write(vals); err != nil {
			return err
		}


	}

*/



	return nil
}



/*
func calcMeanPoints(nodes []*Node, dataFunc func(n *Node) map[float64][]int, key string, ptss map[string]plotter.XYs) {


	stats := make(map[float64][]float64)

	for _, node := range nodes {
		dataStats := dataFunc(node)
		for k, v := range dataStats {
			sum := 0
			for _, intVal := range v {
				sum += intVal
			}
			stats[k] = append(stats[k], float64(sum/len(v)))
		}
	}


	vals := make(map[float64]float64)
	for k,v := range stats {
		vals[k] = godes.Mean(v)
	}

	addPoints(vals, key, ptss)

}*/
/*
	all := make(map[string][]Msg)

	for _, node := range nodes {
		sent := node.GetMessagesSent()
		for key, val := range sent {
			all[key] = append(all[key], val...)
			all["ALL"] = append(all["ALL"], val...)
		}
	}

	fmt.Println("All msg gathered")


	for key, value := range all {

		vals := make(map[float64]float64)

		for _, val := range value {
			vals[val.Time] += 1
		}
		for v := range vals {
			vals[v] = vals[v] / float64(len(nodes))
		}
		addPoints(vals, key, ptss)
	}


	calcMeanPoints(nodes, func(n *Node) map[float64][]int {
		return n.GetTableStats()
	}, "TABLE", ptss)

	calcMeanPoints(nodes, func(n *Node) map[float64][]int {
		return n.GetServerPeersStats()
	}, "Peers", ptss)


	nStats := GetNodeStats()

	vals := make(map[float64]float64)
	for k,v := range nStats {
		sum := 0
		for _, v := range v {
			sum += v
		}

		if sum > 0 {
			vals[k] = float64(sum) / float64(len(v))
		}
	}

	addPoints(vals, "NODES", ptss)

	if common.IsUseGlobalCollector {
		for name, vals := range common.GlobalCollector.CollectMetrics() {
			addPoints(vals, name, ptss)
		}
	} else {
		util.Print("Non-global collector not implemented")
	}


	for name, vals := range common.GlobalCollector.CollectMetrics() {
		addPoints(vals, name, ptss)
	}
*/
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