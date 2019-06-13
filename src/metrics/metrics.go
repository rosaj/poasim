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
}


const(

	PendingTxs = "Pending txs"
	QueuedTxs  = "Queued txs"

)
var TxPoolMetrics = [...]string {
	PendingTxs,
	QueuedTxs,
}

var	(
	DiscoveryTable		= "DiscoveryTable"
)

var DiscoveryMetrics = [...]string{
	PING,
	PONG,
	FINDNODE,
	NEIGHBORS,
	DiscoveryTable,
}

var DevP2PMetrics = [...]string{
	DEVP2P_PING,
	DEVP2P_PONG,
	DEVP2P_HANDSHAKE,
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
