package network

import (
	"../config"
	"../util"

	"errors"
	"fmt"
	"github.com/agoussia/godes"
)

const (

	// Connectivity defaults.
	maxActiveDialTasks     = 16
	defaultMaxPendingPeers = 50
	defaultDialRatio       = 3

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
	DialRatio int `toml:",omitempty"`

	// NoDiscovery can be used to disable the peer discovery mechanism.
	// Disabling is useful for protocol debugging (manual topology).
	NoDiscovery bool

	// BootstrapNodes are used to establish connectivity
	// with the rest of the network.
	BootstrapNodes []*Node

	// Static nodes are used as pre-configured connections which are always
	// maintained and re-connected on disconnects.
	StaticNodes []*Node

	// Trusted nodes are used as pre-configured connections which are always
	// allowed to connect, even above the peer limit.
	TrustedNodes []*Node

	// Protocols should contain the protocols supported
	// by the server. Matching protocols are launched for
	// each peer.
	Protocols []Protocol

	// If NoDial is true, the server will not dial any peers.
	NoDial bool

}

// Server manages all peer connections.
type Server struct {
	// Config fields may not be modified while the server is running.
	Config

	node			*Node

	running 		bool

	ntab        	*Table
	lastLookup  	float64

	peers       	map[ID]*Peer
	inboundCount	int

	refreshFunc 	func()

	pm				*ProtocolManager

	peerStats		map[float64][]int
}



func NewServer(node *Node) *Server {
	return &Server{
		node: node,
		peers: make(map[ID]*Peer),
		peerStats: make(map[float64][]int),
		Config : Config {
			MaxPeers: config.SimConfig.MaxPeers,
			BootstrapNodes: node.bootstrapNodes,
		},
	}
}

func (srv *Server) ProtocolManager() *ProtocolManager {
	return srv.pm
}

// Self returns the local node's endpoint information.
func (srv *Server) Self() *Node {
	return srv.node
}

// Stop terminates the server and all active peer connections.
// It blocks until all active connections have been closed.
func (srv *Server) Stop() {

	if !srv.running {
		return
	}
	srv.running = false

	//TODO: pogleda ovo dali je ok dole
	srv.ntab.Close()

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

	srv.running = true

	srv.pm = NewProtocolManager(srv)
	srv.Protocols = srv.pm.SubProtocols

	srv.setupDiscovery()

	dynPeers := srv.maxDialedConns()
	dialer := newDialState(srv.Self().ID(), srv.StaticNodes, srv.BootstrapNodes, srv.ntab, dynPeers)

	srv.log("DynPeers:", dynPeers)
	srv.run(dialer)
}


func (srv *Server) setupDiscovery() {
	srv.ntab = srv.Self().tab
}


type dialer interface {
	newTasks(running int, peers map[ID]*Peer, now float64) []task
	taskDone(task, float64)
	addStatic(*Node)
	removeStatic(*Node)
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
		trusted      = make(map[ID]bool, len(srv.TrustedNodes))
		//taskdone     = make(chan task, maxActiveDialTasks)
		runningTasks []task
		queuedTasks  []task // tasks that can't run yet
		onTaskDone	 func(task task)
	)
	// Put trusted nodes into a map to speed up checks.
	// Trusted peers are loaded on startup or added via AddTrustedPeer RPC.
	for _, n := range srv.TrustedNodes {
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

	onTaskDone = func(t task) {

		srv.log("Dial task done", t)
		dialstate.taskDone(t, godes.GetSystemTime())
		delTask(t)

		scheduleTasks()
	}

	scheduleTasks()

}



// SetupConn runs the handshakes and attempts to add the connection
// as a peer. It returns when the connection has been added as a peer
// or the handshakes have failed.
func (srv *Server) SetupConn(flags connFlag, node *Node) error {

	peer := NewPeer(srv, node)
	peer.flags = flags
/*
	if srv.trusted[node.ID()] {
		// Ensure that the trusted flag is set before checking against MaxPeers.
		peer.flags |= trustedConn
	}
*/
	err := srv.setupConn(peer)
	if err != nil {
		srv.log("Setting up connection failed", peer, "err", err)
	} else {
		// ako je spojeno i ako smo se mi spojili na node onda posalji connection request da se on spoji na nas
		if !peer.Inbound() {
			node.server.SetupConn(inboundConn, srv.Self())
		}
	}
	return err
}

func (srv *Server) setupConn(peer *Peer) error {

	util.Log(srv)
	if !srv.running {
		return errServerStopped
	}

	err := srv.encHandshakeChecks(peer)

	if err != nil {
		return err
	}

	err = srv.addPeer(peer)
	if err != nil {
		return err
	}

	return nil
}

func (srv *Server) addPeer(peer *Peer) error {

	// At this point the connection is past the protocol handshake.
	// Its capabilities are known and the remote identity is verified.
	err := srv.protoHandshakeChecks(peer)
	if err == nil {

		// The handshakes are done and it passed all checks.
		//p := newPeer(node, srv.Protocols)

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
}

func (srv *Server) FindPeer(node *Node) *Peer {
	return srv.peers[node.ID()]
}



func (srv *Server) protoHandshakeChecks(p *Peer) error {
	// Drop connections with no matching protocols.

	if len(srv.Protocols) > 0 && countMatchingProtocols(srv, p.Server()) == 0 {
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


func countMatchingProtocols(server1 *Server, server2 *Server) int {
	n := 0
	n += 1
	//TODO: ovdje bi mogla bit neka provjera pa da se ne spaja svaki puta na peer-a
	/*
	for _, cap := range caps {
		for _, proto := range protocols {
			if proto.Name == cap.Name && proto.Version == cap.Version {
				n++
			}
		}
	}
	*/
	return n
}

func (srv *Server) log(a ...interface{})  {

	if config.LogConfig.LogServer {
		util.Log("Server:", srv.Self(), a)
	}

}

func (srv *Server) logPeerStats()  {

	t := config.MetricConfig.GetTimeGroup()
	srv.peerStats[t] = append(srv.peerStats[t], len(srv.peers))

}
