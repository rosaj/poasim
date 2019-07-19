package scenario

import (
	"../config"
	"../network"
	"../network/eth"
	"../network/eth/common"
	"../util"
	"github.com/agoussia/godes"
	"math/rand"
	"time"
)


func ScenarioNodeLeavingNetwork(nodes []*network.Node, count int ,duration time.Duration)  {

	util.StartNewRunner(func() {
		godes.Advance( duration.Seconds())


		for count > 0 {
			i := rand.Intn(len(nodes))
			if nodes[i].IsOnline() {
				nodes[i].Kill()
				count--
			}

		}
	})

}




func PrintBlockchainStats(nodes []*network.Node, printAllBlocks bool)  {

	es := nodes[0].Server().(*eth.Ethereum)
	bc := es.BlockChain()

	max := bc.CurrentBlock().NumberU64()

	util.Print("Last block", max)

	for i := 0; i < config.SimConfig.NodeCount ; i+=1  {
		ob := nodes[i].Server().(*eth.Ethereum).BlockChain().CurrentBlock()
		util.Print(nodes[i].Name(), ob.NumberU64())
	}
	size := common.StorageSize(0)
	total := 0
	for i := max;i >= 1 ; i-=1  {
		block := bc.GetBlockByNumber(uint64(i))
		if block != nil {
			signAddr, err := bc.Engine().Author(block.Header())
			if err != nil {
				util.LogError(err)
			}
			signer := findNodeByAddress(nodes, signAddr)
			if printAllBlocks {
				util.Print(block.Number(), "tx count", block.Transactions().Len(), "signer", signer)
			}
			size += block.Size()
			size += block.Header().Size()

			total += len(block.Transactions())
		} else {
			util.Print("block", i, "je nil")
		}
	}

	util.Print("Total num of txs", total)
	util.Print("BC size", size)
}
func findNodeByAddress(nodes []*network.Node, address common.Address) *network.Node {
	for _, node := range nodes {
		if node.Address() == address {
			return node
		}
	}
	return nil
}

