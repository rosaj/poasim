

package network

import (
	"../config"
	"../util"
	"container/heap"
	"errors"
	"fmt"
	"github.com/agoussia/godes"
	"time"
)

const (
	// This is the amount of time spent waiting in between
	// redialing a certain node.
	dialHistoryExpiration = 30 * time.Second

	// Discovery lookups are throttled and can only run
	// once every few seconds.
	lookupInterval = 4 * time.Second

	// If no peers are found for this amount of time, the initial bootnodes are
	// attempted to be connected.
	fallbackInterval = 20 * time.Second

	// Endpoint resolution is throttled with bounded backoff.
	initialResolveDelay = 60 * time.Second
	maxResolveDelay     = time.Hour
)


type dialstate struct {
	maxDynDials int
	ntab        discoverTable

	self        ID

	lookupRunning bool
	dialing       map[ID]connFlag
	lookupBuf     []*Node // current discovery lookup results
	randomNodes   []*Node // filled from Table
	static        map[ID]*dialTask
	hist          *dialHistory

	start     float64     // time when the dialer was first used
	bootnodes []*Node // default dials when there are no peers
}

type discoverTable interface {
	Close()
	Resolve(*Node) *Node
	LookupRandom() []*Node
	ReadRandomNodes([]*Node) int
}

// the dial history remembers recent dials.
type dialHistory []pastDial

// pastDial is an entry in the dial history.
type pastDial struct {
	id  ID
	exp float64
}

type taskRunner struct {
	*godes.Runner
	task task
	server *Server
	taskDone func()
}


func (tr *taskRunner) Run()  {
	tr.task.Do(tr.server)
	tr.taskDone()
}

func RunTask(task task, server *Server, taskDone func())  {
	tr := &taskRunner{&godes.Runner{},task, server, taskDone}
	godes.AddRunner(tr)
}

type task interface {
	Do(*Server)
}

// A dialTask is generated for each node that is dialed. Its
// fields cannot be accessed while the task is running.
type dialTask struct {
	flags        connFlag
	dest         *Node
	lastResolved float64
	resolveDelay float64
}

// discoverTask runs discovery table operations.
// Only one discoverTask is active at any time.
// discoverTask.Do performs a random lookup.
type discoverTask struct {
	results []*Node
}

// A waitExpireTask is generated if there are no other tasks
// to keep the loop in Server.run ticking.
type waitExpireTask struct {
	float64
}

func newDialState(self ID, static []*Node, bootnodes []*Node, ntab discoverTable, maxdyn int) *dialstate {
	s := &dialstate{
		maxDynDials: maxdyn,
		ntab:        ntab,
		self:        self,
		static:      make(map[ID]*dialTask),
		dialing:     make(map[ID]connFlag),
		bootnodes:   make([]*Node, len(bootnodes)),
		randomNodes: make([]*Node, maxdyn/2),
		hist:        new(dialHistory),
	}
	copy(s.bootnodes, bootnodes)
	for _, n := range static {
		s.addStatic(n)
	}
	return s
}

func (s *dialstate) addStatic(n *Node) {
	// This overwrites the task instead of updating an existing
	// entry, giving users the opportunity to force a resolve operation.
	s.static[n.ID()] = &dialTask{flags: staticDialedConn, dest: n}
}

func (s *dialstate) removeStatic(n *Node) {
	// This removes a task so future attempts to connect will not be made.
	delete(s.static, n.ID())
	// This removes a previous dial timestamp so that application
	// can force a server to reconnect with chosen peer immediately.
	s.hist.remove(n.ID())
}

func (s *dialstate) newTasks(nRunning int, peers map[ID]*Peer, now float64) []task {

	if s.start == 0 {
		s.start = godes.GetSystemTime()
	}
	s.log("Running:", nRunning, "peers", len(peers), util.ToDuration(now))

	var newtasks []task

	addDial := func(flag connFlag, n *Node) bool {

		if err := s.checkDial(n, peers); err != nil {
			s.log("Skipping dial candidate", "id", n, "err", err)
			return false
		}

		s.dialing[n.ID()] = flag

		newtasks = append(newtasks, &dialTask{flags: flag, dest: n})

		return true
	}


	// Compute number of dynamic dials necessary at this point.
	needDynDials := s.maxDynDials
	for _, p := range peers {
		if p.is(dynDialedConn) {
			needDynDials--
		}
	}
	for _, flag := range s.dialing {
		if flag&dynDialedConn != 0 {
			needDynDials--
		}
	}

	// Expire the dial history on every invocation.
	s.hist.expire(now)

	// Create dials for static nodes if they are not connected.
	for id, t := range s.static {
		err := s.checkDial(t.dest, peers)

		switch err {
			case errNotWhitelisted, errSelf:

				s.log("Removing static dial candidate", "id", t.dest, "err", err)
				delete(s.static, t.dest.ID())
			case nil:
				s.dialing[id] = t.flags
				newtasks = append(newtasks, t)
			}
	}



	// If we don't have any peers whatsoever, try to dial a random bootnode. This
	// scenario is useful for the testnet (and private networks) where the discovery
	// table might be full of mostly bad peers, making it hard to find good ones.
	if len(peers) == 0 && len(s.bootnodes) > 0 && needDynDials > 0 && godes.GetSystemTime() - s.start > fallbackInterval.Seconds() {

		bootnode := s.bootnodes[0]

		s.bootnodes = append(s.bootnodes[:0], s.bootnodes[1:]...)
		s.bootnodes = append(s.bootnodes, bootnode)

		if addDial(dynDialedConn, bootnode) {
			needDynDials--
		}

	}


	// Use random nodes from the table for half of the necessary
	// dynamic dials.
	randomCandidates := needDynDials / 2

	if randomCandidates > 0 {
		n := s.ntab.ReadRandomNodes(s.randomNodes)
		for i := 0; i < randomCandidates && i < n; i++ {
			if addDial(dynDialedConn, s.randomNodes[i]) {
				needDynDials--
			}
		}
	}

	// Create dynamic dials from random lookup results, removing tried
	// items from the result buffer.
	i := 0

	for ; i < len(s.lookupBuf) && needDynDials > 0; i++ {
		if addDial(dynDialedConn, s.lookupBuf[i]) {
			needDynDials--
		}
	}

	s.lookupBuf = s.lookupBuf[:copy(s.lookupBuf, s.lookupBuf[i:])]

	// Launch a discovery lookup if more candidates are needed.
	if len(s.lookupBuf) < needDynDials && !s.lookupRunning {
		s.lookupRunning = true
		newtasks = append(newtasks, &discoverTask{})
	}

	// Launch a timer to wait for the next node to expire if all
	// candidates have been tried and no task is currently active.
	// This should prevent cases where the dialer logic is not ticked
	// because there are no pending events.
	if nRunning == 0 && len(newtasks) == 0 && s.hist.Len() > 0 {
		dif := s.hist.min().exp - now
		if dif > 0 {
			t := &waitExpireTask{dif}
			newtasks = append(newtasks, t)
		}
	}

	return newtasks
}

var (
	errSelf             = errors.New("is self")
	errAlreadyDialing   = errors.New("already dialing")
	errAlreadyConnected = errors.New("already connected")
	errRecentlyDialed   = errors.New("recently dialed")
	errNotWhitelisted   = errors.New("not contained in netrestrict whitelist")
)

func (s *dialstate) checkDial(n *Node, peers map[ID]*Peer) error {
	_, dialing := s.dialing[n.ID()]
	switch {
	case dialing:
		return errAlreadyDialing
	case peers[n.ID()] != nil:
		return errAlreadyConnected
	case n.ID() == s.self:
		return errSelf
	case s.hist.contains(n.ID()):
		return errRecentlyDialed
	}
	return nil
}

func (s *dialstate) taskDone(t task, now float64) {
	switch t := t.(type) {
		case *dialTask:
			s.hist.add(t.dest.ID(), now + dialHistoryExpiration.Seconds())
			delete(s.dialing, t.dest.ID())
		case *discoverTask:
			s.lookupRunning = false
			s.lookupBuf = append(s.lookupBuf, t.results...)
	}
}

func (t *dialTask) Do(srv *Server) {

	err := t.dial(srv, t.dest)
	if err != nil {
		//log.Trace("Dial error", "task", t, "err", err)
		// Try resolving the ID of static nodes if dialing failed.
		if _, ok := err.(*dialError); ok && t.flags&staticDialedConn != 0 {
			if t.resolve(srv) {
				t.dial(srv, t.dest)
			}
		}
	}
}

// resolve attempts to find the current endpoint for the destination
// using discovery.
//
// Resolve operations are throttled with backoff to avoid flooding the
// discovery network with useless queries for nodes that don't exist.
// The backoff delay resets when the node is found.
func (t *dialTask) resolve(srv *Server) bool {
	if srv.ntab == nil {
		return false
	}

	if t.resolveDelay == 0 {
		t.resolveDelay = initialResolveDelay.Seconds()
	}


	if t.lastResolved < t.resolveDelay {
		return false
	}

	resolved := srv.ntab.Resolve(t.dest)

	t.lastResolved = godes.GetSystemTime()

	if resolved == nil {

		t.resolveDelay *= 2

		if t.resolveDelay > maxResolveDelay.Seconds() {
			t.resolveDelay = maxResolveDelay.Seconds()
		}

		//log.Debug("Resolving node failed", "id", t.dest, "newdelay", t.resolveDelay)
		return false
	}

	// The node was found.
	t.resolveDelay = initialResolveDelay.Seconds()
	t.dest = resolved

	//log.Debug("Resolved node", "id", t.dest)

	return true
}

type dialError struct {
	error
}

// dial performs the actual connection attempt.
func (t *dialTask) dial(srv *Server, dest *Node) error {
	godes.Advance(config.SimConfig.NextNetworkLatency())
	return srv.SetupConn(t.flags, dest)
}


func (t *dialTask) String() string {
	return fmt.Sprintf("%v %x", t.flags, t.dest)
}



func (t *discoverTask) Do(srv *Server) {
	// newTasks generates a lookup task whenever dynamic dials are
	// necessary. Lookups need to take some time, otherwise the
	// event loop spins too fast.
	next := srv.lastLookup + lookupInterval.Seconds()

	if now := godes.GetSystemTime(); now < next {
		godes.Advance(next-now)
	}
	srv.lastLookup = godes.GetSystemTime()
	t.results = srv.ntab.LookupRandom()
}

func (t *discoverTask) String() string {
	s := "discovery lookup"
	if len(t.results) > 0 {
		s += fmt.Sprintf(" (%d results)", len(t.results))
	}
	return s
}

func (t waitExpireTask) Do(*Server) {
	godes.Advance(t.float64)

}
func (t waitExpireTask) String() string {
	return fmt.Sprintf("wait for dial hist expire (%v)", t.float64)
}

// Use only these methods to access or modify dialHistory.
func (h dialHistory) min() pastDial {
	return h[0]
}
func (h *dialHistory) add(id ID, exp float64) {
	heap.Push(h, pastDial{id, exp})

}
func (h *dialHistory) remove(id ID) bool {
	for i, v := range *h {
		if v.id == id {
			heap.Remove(h, i)
			return true
		}
	}
	return false
}
func (h dialHistory) contains(id ID) bool {
	for _, v := range h {
		if v.id == id {
			return true
		}
	}
	return false
}
func (h *dialHistory) expire(now float64) {
	for h.Len() > 0 && h.min().exp < now {
		heap.Pop(h)
	}
}

// heap.Interface boilerplate
func (h dialHistory) Len() int           { return len(h) }
func (h dialHistory) Less(i, j int) bool { return h[i].exp < (h[j].exp) }
func (h dialHistory) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *dialHistory) Push(x interface{}) {
	*h = append(*h, x.(pastDial))
}
func (h *dialHistory) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func (d *dialstate) log(a ...interface{})  {
	util.Log("Dialstate:", a)
}

