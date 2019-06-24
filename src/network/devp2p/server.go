package devp2p

import (
	. "../../common"
	. "../../config"
	"../../metrics"
	. "../../util"
	"errors"
	"fmt"
	"github.com/agoussia/godes"
)

const (

	// Connectivity defaults.
	maxActiveDialTasks     = 16
	defaultMaxPendingPeers = 50
	defaultDialRatio       = 3

	PeerCount = metrics.DEVp2pPeers
	DiscPeers = metrics.DEVp2pDisconnectedPeers
)


var errServerStopped = errors.New("server stopped")

// Config holds Server options.
type Config struct {

	// MaxPeers is the maximum number of peers that can be
	// connected. It must be greater than zero.
	MaxPeers int

	// DialRatio controls the ratio of inbound to dialed connections.
	// Example: a DialRatio of 2 allows 1/2 of connections to be dialed.
	// Setting DialRatio to zero defaults it to 3.
	DialRatio int

	// NoDiscovery can be used to disable the peer discovery mechanism.
	// Disabling is useful for protocol debugging (manual topology).
	NoDiscovery bool

	// BootNodes are used to establish connectivity
	// with the rest of the network.
	BootNodes []INode

	// Static INodes are used as pre-configured connections which are always
	// maintained and re-connected on disconnects.
	StaticINodes []INode

	// Trusted INodes are used as pre-configured connections which are always
	// allowed to connect, even above the peer limit.
	TrustedINodes []INode

	// Protocols should contain the protocols supported
	// by the server. Matching protocols are launched for
	// each peer.
	Protocols []IProtocol

	// If NoDial is true, the server will not dial any peers.
	NoDial bool

}

// Server manages all peer connections.
type Server struct {
	// Config fields may not be modified while the server is running.
	Config
	IMetricCollector

	node			INode

	running 		bool

	ntab       IDiscoveryTable
	lastLookup float64

	peers       	map[ID]*Peer
	handshakePeers  map[ID]*Peer
	inboundCount	int

	refreshFunc 	func()
	quitFunc		func()

}


func NewServer(node INode, metricCollector IMetricCollector) *Server {
	srv :=  &Server{
				IMetricCollector: metricCollector,
				node: node,
				peers: make(map[ID]*Peer),
				handshakePeers: make(map[ID]*Peer),

				Config : Config {
					MaxPeers: node.GetMaxPeers(),
					DialRatio: node.GetDialRatio(),
					BootNodes: node.GetBootNodes(),
				},
			}

	return srv
}


// Self returns the local INode's endpoint information.
func (srv *Server) Self() INode {
	return srv.node
}


func (srv *Server) GetProtocols() []IProtocol {
	return srv.Protocols
}
func (srv *Server) GetProtocolManager() IProtocolManager {
	return nil
}
/*
func (srv *Server) GetProtocolManager() IProtocolManager {
	return srv.pm
}
*/
func (srv *Server) SetOnline(online bool)  {

	if online {
		srv.Start()
	} else {
		srv.Stop()
	}

}

// Stop terminates the server and all active peer connections.
// It blocks until all active connections have been closed.
func (srv *Server) Stop() {

	if !srv.running {
		return
	}
	srv.log("Stopping...")

	srv.running = false

	if srv.quitFunc != nil {
		srv.quitFunc()
	}

	// Disconnect all peers.
	for _, p := range srv.peers {
		p.Disconnect(DiscQuitting)
	}

}

// Start starts running the server.
// Servers can not be re-used after stopping.
func (srv *Server) Start() {

	if srv.running {
		return
	}

	srv.log("Starting...")

	srv.running = true

	srv.setupDiscovery()

	dynPeers := srv.maxDialedConns()
	dialer := newDialState(srv.Self().ID(), srv.StaticINodes, srv.BootNodes, srv.ntab, dynPeers)

	srv.log("DynPeers:", dynPeers)
	srv.run(dialer)

}


func (srv *Server) setupDiscovery() {
	srv.ntab = srv.Self().GetDiscoveryTable()
}


type dialer interface {
	newTasks(running int, peers map[ID]*Peer, now float64) []task
	taskDone(task, float64)
	addStatic(INode)
	removeStatic(INode)
}



// pobuduje taskove povezivanja sa peerovima
// kad se bilo koja promjena napravi ca se tice spojenih peer-ova
// potrebno je pozvat ovu metodu
func (srv *Server) Refresh()  {
	if srv.refreshFunc != nil {
		srv.refreshFunc()
	}
}

func (srv *Server) run(dialstate dialer) {
	srv.log("Started P2P networking")

	var (
		trusted      = make(map[ID]bool, len(srv.TrustedINodes))
		//taskdone     = make(chan task, maxActiveDialTasks)
		runningTasks []task
		queuedTasks  []task // tasks that can't run yet
		onTaskDone	 func(task task)
	)
	// Put trusted INodes into a map to speed up checks.
	// Trusted peers are loaded on startup or added via AddTrustedPeer RPC.
	for _, n := range srv.TrustedINodes {
		trusted[n.ID()] = true
	}

	// removes t from runningTasks
	delTask := func(t task) {
		for i := range runningTasks {
			if runningTasks[i] == t {
				runningTasks = append(runningTasks[:i], runningTasks[i+1:]...)
				break
			}
		}
	}
	// starts until max number of active tasks is satisfied
	startTasks := func(ts []task) (rest []task) {
		i := 0
		for ; len(runningTasks) < maxActiveDialTasks && i < len(ts); i++ {
			t := ts[i]
			srv.log("New dial task", t)

			RunTask(t, srv, func() {
				onTaskDone(t)
			})

			runningTasks = append(runningTasks, t)
		}
		return ts[i:]
	}
	scheduleTasks := func() {
		// Start from queue first.
		queuedTasks = append(queuedTasks[:0], startTasks(queuedTasks)...)
		// Query dialer for new tasks and start as many as possible now.
		if len(runningTasks) < maxActiveDialTasks {
			nt := dialstate.newTasks(len(runningTasks)+len(queuedTasks), srv.peers, godes.GetSystemTime())
			srv.log("New tasks", len(nt))
			queuedTasks = append(queuedTasks, startTasks(nt)...)
		}
	}

	srv.refreshFunc = func() {
		scheduleTasks()
	}

	srv.quitFunc = func() {
		srv.refreshFunc = nil

		for _, task := range runningTasks {
			StopTask(task)
		}
	}


	onTaskDone = func(t task) {

		if srv.running {

			srv.log("Dial task done", t)
			dialstate.taskDone(t, godes.GetSystemTime())
			delTask(t)

			scheduleTasks()

		}
	}

	scheduleTasks()

}



// SetupConn runs the handshakes and attempts to add the connection
// as a peer. It returns when the connection has been added as a peer
// or the handshakes have failed.
func (srv *Server) SetupConn(flags connFlag, node INode) error {
	if node.Server() == nil {
		return nil
	}

	peer := NewPeer(srv, node)
	peer.flags = flags
/*
	if srv.trusted[node.ID()] {
		// Ensure that the trusted flag is set before checking against MaxPeers.
		peer.flags |= trustedConn
	}
*/
	srv.setupConn(peer, func(err error) {
		if err != nil {
			srv.log("Setting up connection failed", peer, "err", err)
		}
	})

	// ako smo se mi spojili na INode onda posalji connection request da se on spoji na nas
	if !peer.Inbound() {
		node.Server().SetupPeerConn(int32(inboundConn), srv.Self())
	}



	return nil
}

func (srv *Server) SetupPeerConn(flags int32, node INode) error {
	cf := connFlag(flags)
	return srv.SetupConn(cf, node)
}

func (srv *Server) setupConn(peer *Peer, onError func(err error)) {

	if !srv.running {
		onError(errServerStopped)
		return
	}

	err := srv.encHandshakeChecks(peer)

	if err != nil {
		onError(err)
		return
	}

	srv.handshakePeers[peer.ID()] = peer

	srv.log("Handshake peer", peer)

	// Run the protocol handshake
	peer.doProtoHandshake(func(err error) {

		if err != nil {
			onError(err)
			return
		}

		err = srv.addPeer(peer)
		if err != nil {
			onError(err)
			return
		}

	})

}

func (srv *Server) RetrieveHandshakePeer(node INode) IPeer {
	defer delete(srv.handshakePeers, node.ID())
	return srv.handshakePeers[node.ID()]
}


func (srv *Server) addPeer(peer *Peer) error {

	// At this point the connection is past the protocol handshake.
	// Its capabilities are known and the remote identity is verified.
	err := srv.protoHandshakeChecks(peer)
	if err == nil {

		// The handshakes are done and it passed all checks.
		//p := newPeer(INode, srv.Protocols)

		srv.log("Adding p2p peer", peer, "peers", len(srv.peers)+1)

		peer.run()

		srv.peers[peer.ID()] = peer
		if peer.Inbound() {
			srv.inboundCount++
		}
	}

	srv.Refresh()

	srv.logPeerStats()

	return err
}

func (srv *Server) DeletePeer(p *Peer)  {

	if srv.peers[p.ID()] == nil {
		return
	}

	delete(srv.peers, p.ID())

	if p.Inbound() {
		srv.inboundCount--
	}

	srv.Refresh()

	srv.logPeerStats()

	srv.Update(DiscPeers)
}

func (srv *Server) FindPeer(node INode) IPeer {
	return srv.peers[node.ID()]
}
func (srv *Server) PeerCount() int {
	return len(srv.peers)
}


func (srv *Server) protoHandshakeChecks(p *Peer) error {
	// Drop connections with no matching protocols.

	if len(srv.Protocols) > 0 && srv.countMatchingProtocols(p) == 0 {
		return DiscUselessPeer
	}
	// Repeat the encryption handshake checks because the
	// peer set might have changed between the handshakes.
	return srv.encHandshakeChecks(p)
}

func (srv *Server) encHandshakeChecks(p *Peer) error {
	switch {
	case !p.is(trustedConn|staticDialedConn) && len(srv.peers) >= srv.MaxPeers:
		return DiscTooManyPeers
	case !p.is(trustedConn) && p.is(inboundConn) && srv.inboundCount >= srv.maxInboundConns():
		return DiscTooManyPeers
	case srv.peers[p.ID()] != nil:
		return DiscAlreadyConnected
	case p.ID() == srv.Self().ID():
		return DiscSelf
	default:
		return nil
	}
}

func (srv *Server) maxInboundConns() int {
	return srv.MaxPeers - srv.maxDialedConns()
}
func (srv *Server) maxDialedConns() int {
	if srv.NoDiscovery || srv.NoDial {
		return 0
	}
	r := srv.DialRatio
	if r == 0 {
		r = defaultDialRatio
	}
	return srv.MaxPeers / r
}

func (srv *Server) String() string {
	return fmt.Sprintf("Server %s", srv.Self().Name())
}


func (srv *Server) countMatchingProtocols(p *Peer) int {
	n := 0

	for _, proto1 := range srv.Protocols {
		for _, proto2 := range p.Node().Server().GetProtocols() {
			if proto1.GetName() == proto2.GetName() {
				n++
			}
		}
	}

	return n
}

func (srv *Server) log(a ...interface{})  {

	if LogConfig.LogServer {
		Log("Server:", srv.Self(), a)
	}

}

func (srv *Server) logPeerStats()  {
	srv.Set(PeerCount, len(srv.peers))

}
