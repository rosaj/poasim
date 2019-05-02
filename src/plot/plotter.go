package plot

import (
	"../config"
	"../network"
	"../util"
	"fmt"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
	"math"
	"math/rand"
	"sort"
	"strconv"
)


func Stats(nodes []*network.Node)  {

	all := make(map[string][]network.Msg)

	for _, node := range nodes {
		sent := node.GetMessagesSent()
		for key, val := range sent {
			all[key] = append(all[key], val...)
			all["ALL"] = append(all["ALL"], val...)
		}
		received := node.GetMessagesReceived()

		for key, val := range received {
			all[key+"_RECEIVED"] = append(all[key+"_RECEIVED"], val...)
			all["ALL_RECEIVED"] = append(all["ALL_RECEIVED"], val...)
		}
	}

	fmt.Println("All msg gathered")
	p, err := plot.New()
	if err != nil {
		panic(err)
	}

	p.Title.Text = fmt.Sprintf("Messages, nodes: %d, node arrival every: %s, grouped every: %s",
		config.SimConfig.NodeCount,
		util.ToDuration(config.SimConfig.NextNodeArrival()).String(),
		util.ToDuration(config.MetricConfig.MsgGroupFactor).String())

	p.X.Label.Text = "Time"
	p.Y.Label.Text = "Message count"


	ptss := make(map[string]plotter.XYs)

	for key, value := range all {

		vals := make(map[float64]float64)

		for _, val := range value {
			vals[val.Time] += 1
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
		ptss[key] = pts

	}

	fmt.Println("Points prepared")

	err = plotutil.AddLines(p,
		"Ping", ptss["PING"],
		"Pong", ptss["PONG"],
		"FindNode", ptss["FINDNODE"],
		"Neighbors", ptss["NEIGHBORS"],
		"SEND_ERR", ptss["SEND_ERR"],
		"Ping_Received", ptss["PING_RECEIVED"],
		"Pong_Received", ptss["PONG_RECEIVED"],
		"FindNode_Received", ptss["FINDNODE_RECEIVED"],
		"Neighbors_Received", ptss["NEIGHBORS_RECEIVED"],
		"RECIEVE_ERR", ptss["RECIEVE_ERR"])

	if err != nil {
		panic(err)
	}

	temp := ptss["ALL"]
	xNames := make([]string, len(temp))

	for i, val := range temp {
		if math.Mod(float64(i), float64(len(xNames)/10)) == 0{
			xNames[i] = util.ToDuration(val.X * config.MetricConfig.MsgGroupFactor).String()
		}
	}

	p.NominalX(xNames...)

	fmt.Println("X names set")

	name := fmt.Sprintf("msg_%s_%s_.png",
		util.ToDuration(config.SimConfig.SimulationTime).String(),
		strconv.Itoa(config.SimConfig.NodeCount))

	// Save the plot to a PNG file.
	if err := p.Save(vg.Length(1280), vg.Length(768), name); err != nil {
		panic(err)
	}
	fmt.Println("wrote to", name)

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

	// Save the plot to a PNG file.
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