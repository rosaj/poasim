package generate

import (
	. "../config"
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

var	nonceCounter = make(map[common.Address]uint64)

func txs(broadcastNodes []*network.Node,  actorCount int, stepFunc func(count int) (bool, float64))  {

	actors := core.Actors
	actorsAddrs := make([]common.Address, 0)
	for addr := range actors {
		actorsAddrs = append(actorsAddrs, addr)
	}

	count := 0

	for next, step := stepFunc(count); next; {

		nextActor := rand.Intn(actorCount)

		addr := actorsAddrs[nextActor]

		tx := newTransaction(actors[addr], core.BankAddress, nonceCounter[addr], big.NewInt(1))
		nonceCounter[addr]+=1

		randomBroadcast(broadcastNodes, append(make(types.Transactions, 0), tx))

		if step > 0 {
			godes.Advance(step)
		}

		count += 1
	}

}


func Txs(broadcastNodes []*network.Node, actorCount int, txCount int, step float64)  {

	txs(broadcastNodes, actorCount, func(count int) (bool, float64) {
		return count < txCount, step
	})

/*
	//actors := generateActors(actorCount)
	actors := core.Actors
	actorsAddrs := make([]common.Address, 0)
	for addr := range actors {
		actorsAddrs = append(actorsAddrs, addr)
	}

//	donate(broadcastNodes, actorsAddrs)

	for i := 0; i < txCount; i += 1 {
		nextActor := rand.Intn(actorCount)

		addr := actorsAddrs[nextActor]

		tx := newTransaction(actors[addr], core.BankAddress, nonceCounter[addr], big.NewInt(1))
		nonceCounter[addr]+=1

		randomBroadcast(broadcastNodes, append(make(types.Transactions, 0), tx))

		if step > 0 {
			godes.Advance(step)
		}
	}
*/
}

func TxsDistr(broadcastNodes []*network.Node, actorCount int)  {

	txs(broadcastNodes, actorCount, func(count int) (bool, float64) {
		return !SimConfig.SimulationEnded(), SimConfig.NextTrInterval()
	})
/*
	actors := core.Actors
	actorsAddrs := make([]common.Address, 0)
	for addr := range actors {
		actorsAddrs = append(actorsAddrs, addr)
	}


	for !SimConfig.SimulationEnded()  {

		nextActor := rand.Intn(actorCount)

		addr := actorsAddrs[nextActor]

		tx := newTransaction(actors[addr], core.BankAddress, nonceCounter[addr], big.NewInt(1))
		nonceCounter[addr]+=1

		randomBroadcast(broadcastNodes, append(make(types.Transactions, 0), tx))

		step := SimConfig.NextTrInterval()
		if step > 0 {
			godes.Advance(step)
		}
	}
*/
}

func AsyncTxs(broadcastNodes []*network.Node, actorCount int, txCount int, step float64)  {
	util.StartNewRunner(func() {
		Txs(broadcastNodes, actorCount, txCount, step)
	})
}




func randomBroadcast(broadcastNodes []*network.Node, txs types.Transactions)  {
	index := rand.Intn(len(broadcastNodes))
	errors := broadcastNodes[index].Server().GetProtocolManager().AddTxs(txs)
	//util.Log("sending to ", broadcastNodes[index].Name())
	for _, err := range errors {
		if err != nil {
			util.LogError(err)
		}
	}
/*
	for _, node := range broadcastNodes {
		fmt.Print(node.Name(), " pending txs ", node.Server().GetProtocolManager().PendingTxCount(), " ")
	}
	fmt.Println()
 */
}


func newTransaction(from *ecdsa.PrivateKey, to common.Address, nonce uint64, amount	*big.Int) *types.Transaction {
	tx := types.NewTransaction(nonce, to, amount, 100000, big.NewInt(1), nil)
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