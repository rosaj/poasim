package generate

import (
	. "../common"
	. "../config"
	"../metrics"
	"../network"
	"../network/eth/common"
	"../network/eth/core"
	"../network/eth/core/types"
	"../util"
	"crypto/ecdsa"
	"github.com/agoussia/godes"
	"math/big"
	"math/rand"
)
var txStats = GlobalMetricCollector

func GetTxsStats() map[float64]float64 {
	return txStats.Collect(metrics.TxsArrival)
}


var	nonceCounter = make(map[common.Address]uint64)

func Reset()  {
	nonceCounter = make(map[common.Address]uint64)
	GlobalMetricCollector = NewMetricCollector()
	txStats = GlobalMetricCollector
}


func txs(broadcastNodes []*network.Node,  actorCount int, stepFunc func(count int) (bool, float64))  {

	actors := core.Actors
	actorsAddrs := make([]common.Address, 0)
	for addr := range actors {
		actorsAddrs = append(actorsAddrs, addr)
	}

	count, errCount := 0, 0
	next, step := stepFunc(count)
	for  next {

		nextActor := rand.Intn(actorCount)

		addr := actorsAddrs[nextActor]

		tx := newTransaction(actors[addr], core.BankAddress, nonceCounter[addr], big.NewInt(1))
		nonceCounter[addr]+=1

		if errors := randomBroadcast(broadcastNodes, append(make(types.Transactions, 0), tx)); errors > 0 {
			nonceCounter[addr]-=1
			errCount += errCount
		}


		if step > 0 {
			godes.Advance(step)
		}

		count += 1
		next, step = stepFunc(count)
	}

	util.Print("Generated", count, "txs", "errors", errCount, "final:", count-errCount)
}


func Txs(broadcastNodes []*network.Node, actorCount int, txCount int, step float64)  {

	txs(broadcastNodes, actorCount, func(count int) (bool, float64) {
		return count < txCount, step
	})
}

func TxsDistr(broadcastNodes []*network.Node)  {

	startTime := godes.GetSystemTime()

	txGenConfig := SimConfig.TxGeneratorConfig
	txs(broadcastNodes, txGenConfig.ActorCount, func(count int) (bool, float64) {
		return util.TimeSince(uint64(startTime)) < txGenConfig.Duration, txGenConfig.NextTrInterval()
	})
}

func AsyncTxsDistr(broadcastNodes []*network.Node)  {
	util.StartNewRunner(func() {
		TxsDistr(broadcastNodes)
	})
}

func AsyncTxs(broadcastNodes []*network.Node, actorCount int, txCount int, step float64)  {
	util.StartNewRunner(func() {
		Txs(broadcastNodes, actorCount, txCount, step)
	})
}




func randomBroadcast(broadcastNodes []*network.Node, txs types.Transactions) int {


	index := rand.Intn(len(broadcastNodes))

	for !broadcastNodes[index].IsOnline() {
		index = rand.Intn(len(broadcastNodes))
	}

	txStats.Update(metrics.TxsArrival)
	FinalityMetricCollector.TxSubmitted(txs[0].Hash().String())

	errors := broadcastNodes[index].Server().GetProtocolManager().AddTxs(txs)
	//util.Log("sending to ", broadcastNodes[index].Name())

	errCount := 0

	for _, err := range errors {

		if err != nil {
			errCount += 1
			util.LogError(err)
		}
	}

	return errCount

}

func newTransaction(from *ecdsa.PrivateKey, to common.Address, nonce uint64, amount	*big.Int) *types.Transaction {
	txPrice := SimConfig.TxGeneratorConfig.NextTxPrice()
	tx := types.NewTransaction(nonce, to, amount, 100000, big.NewInt(int64(txPrice)), nil)
	tx, _ = types.SignTx(tx, types.HomesteadSigner{}, from)
	return tx
}



/*
func donate(broadcastNodes []*network.Node, to []common.Address)  {
	//money := new(big.Int).Div(core.BankFunds, big.NewInt(int64(len(to)*2)))
	money := big.NewInt(params.Ether)
	txs := make(types.Transactions,0)

	startNonce := nonceCounter[core.BankAddress]

	for nonce, addr := range to {
		tx := newTransaction(core.BankKey, addr, uint64(nonce)+startNonce, money)
		txs = append(txs, tx)
	}

	nonceCounter[core.BankAddress] += uint64(len(to))

	randomBroadcast(broadcastNodes, txs)
	godes.Advance(300)
}

func generateActors(count int) map[common.Address]*ecdsa.PrivateKey {
	actors := make(map[common.Address]*ecdsa.PrivateKey, 0)

	for i := 0; i < count; i += 1 {
		key := network.NewKey()
		addr := common.Address(crypto.PubkeyToAddress(key.PublicKey))
		actors[addr] = key
	}

	return actors
}

*/