package metrics

const (
	NonContiguousInsert 		= "non contiguous insert"
	NonContiguousReceiptInsert 	= "non contiguous receipt insert"
	BadBlock            		= "bad block"
	InsertNewBlock      		= "insert new block"
	InsertForkedBlock   		= "insert forked block"
	SidechainDetected   		= "sidechain detected"
	SidechainInject     		= "sidechain inject"
	MissingParent       		= "missing parent"
	ChainSplitDetected  		= "chain split detected"
	ChainSplitDepth				= "chain split depth"
	GasLimit					= "gas limit"
	TxsPerBlock					= "txs per block"
	SyncDiff					= "sync dif"
)


var	BlockchainMetrics = [...]string{
	NonContiguousInsert,
	NonContiguousReceiptInsert,
	BadBlock,
	InsertNewBlock,
	InsertForkedBlock,
	SidechainDetected,
	SidechainInject,
	MissingParent,
	ChainSplitDetected,
	ChainSplitDepth,
	GasLimit,
	TxsPerBlock,

}


const(

	PendingTxs = "Pending txs"
	QueuedTxs  = "Queued txs"
	TxsArrival = "Txs arrival"

)
var TxPoolMetrics = [...]string {
	PendingTxs,
	QueuedTxs,
	TxsArrival,
}

const (
	TxPoolErrors 			= "Tx pool errors"
	TransactionUnderpriced 	= "transaction underpriced"
	InsufficientFunds		= "insufficient funds for gas * price + value"
	NonceTooLow				= "nonce too low"
	ExceedsGasLimit 		= "exceeds block gas limit"
	KnownTransaction		= "known transaction"
)

var TxPoolErrorMetrics = [...]string {
	TxPoolErrors,
	TransactionUnderpriced,
	InsufficientFunds,
	NonceTooLow,
	ExceedsGasLimit,
	KnownTransaction,
}

var AllTxPoolMetrics = append(TxPoolMetrics[:], TxPoolErrorMetrics[:]...)


var	(
	DiscoveryTable		= "DiscoveryTable"
	OnlineNodes 		= "Online nodes"

)

var DiscoveryMetrics = [...]string{
	PING,
	PONG,
	FINDNODE,
	NEIGHBORS,
	DiscoveryTable,
	OnlineNodes,

}

const (
	DEVp2pPeers 			= "DEVp2p peers"
	DEVp2pDisconnectedPeers = "Disconnected peers"
	DialTask				= "Dial task"
	DiscoveryTask			= "Discovery task"

)



var DevP2PMetrics = [...]string{
	DEVP2P_PING,
	DEVP2P_PONG,
	DEVP2P_HANDSHAKE,
	DEVp2pPeers,
	DialTask,
	DiscoveryTask,
	DEVp2pDisconnectedPeers,
}

const 	(
	EthPeers = "Eth peers"
	MinedBlock = "Mined block"
)

var EthProtoMetrics = [...]string{
	STATUS_MSG,
	NEW_BLOCK_HASHES_MSG,
	TX_MSG,
	GET_BLOCK_HEADERS_MSG,
	BLOCK_HEADERS_MSG,
	NEW_BLOCK_MSG,
	EthPeers,
	MinedBlock,
}

var (
	PING		= "PING"
	PONG		= "PONG"
	FINDNODE	= "FINDNODE"
	NEIGHBORS	= "NEIGHBORS"


	DEVP2P_HANDSHAKE 	= "DEVP2P_HANDSHAKE"
	DEVP2P_PING			= "DEVP2P_PING"
	DEVP2P_PONG			= "DEVP2P_PONG"


	// eth protocol message codes

	STATUS_MSG				= "StatusMsg"
	NEW_BLOCK_HASHES_MSG	= "NewBlockHashesMsg"
	TX_MSG					= "TxMsg"
	GET_BLOCK_HEADERS_MSG	= "GetBlockHeadersMsg"
	BLOCK_HEADERS_MSG		= "BlockHeadersMsg"
	NEW_BLOCK_MSG			= "NewBlockMsg"

)


var AllMetrics = append(AllTxPoolMetrics[:],
						append(BlockchainMetrics[:],
							append(EthProtoMetrics[:],
								append(DevP2PMetrics[:], DiscoveryMetrics[:]...
									)...
								)...
							)...
						)


