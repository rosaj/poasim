package network

import (
	"../util"
	crand "crypto/rand"
	"encoding/binary"
	"github.com/agoussia/godes"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"math/bits"
	mrand "math/rand"
	"sort"
	"time"
)

const (
	alpha           = 3  // Kademlia concurrency factor
	bucketSize      = 16 // Kademlia bucket size
	maxReplacements = 10 // Size of per-bucket replacement list

	// We keep buckets for the upper 1/15 of distances because
	// it's very unlikely we'll ever encounter a node that's closer.
	hashBits          = len(common.Hash{}) * 8
	nBuckets          = hashBits / 15       // Number of buckets
	bucketMinDistance = hashBits - nBuckets // Log distance of closest bucket

	maxFindnodeFailures = 5 // Nodes exceeding this limit are dropped
	refreshInterval     = 30 * time.Minute
	revalidateInterval  = 10 * time.Second
	copyNodesInterval   = 30 * time.Second
	seedMinTableTime    = 5 * time.Minute
	seedCount           = 30
	seedMaxAge          = 5 * 24 * time.Hour
)

type Table struct {
	buckets [nBuckets]*bucket // index of known nodes by distance
	nursery []*Node           // bootstrap nodes
	rand    *mrand.Rand       // source of randomness, periodically reseeded
	db *DB
	net        *udp

	refreshDone godes.BooleanControl

	tableRunners []*tableRunner

}

// bucket contains nodes, ordered by their last activity. the entry
// that was most recently active is the first element in entries.
type bucket struct {
	entries      []*Node // live entries, sorted by time of last contact
	replacements []*Node // recently seen nodes to be used if revalidation fails
}

func newTable(net *udp, bootstrapNodes []*Node) *Table {
	tab := &Table{
		net:        net,
		db:			net.db,
		rand:       mrand.New(mrand.NewSource(0)),
	}

	tab.nursery = bootstrapNodes

	tab.goOnline()

	return tab
}

func (tab *Table) log(s ...interface{})  {
	util.Log(tab.self().Name(), s)
}

func (tab *Table) self() *Node {
	return tab.net.self()
}

func (tab *Table) seedRand() {
	var b [8]byte
	crand.Read(b[:])
	tab.rand.Seed(int64(binary.BigEndian.Uint64(b[:])))
}

// ReadRandomNodes fills the given slice with random nodes from the table. The results
// are guaranteed to be unique for a single invocation, no node will appear twice.
func (tab *Table) ReadRandomNodes(buf []*Node) (n int) {

	// Find all non-empty buckets and get a fresh slice of their entries.
	var buckets [][]*Node
	for _, b := range &tab.buckets {
		if len(b.entries) > 0 {
			buckets = append(buckets, b.entries)
		}
	}
	if len(buckets) == 0 {
		return 0
	}
	// Shuffle the buckets.
	for i := len(buckets) - 1; i > 0; i-- {
		j := tab.rand.Intn(len(buckets))
		buckets[i], buckets[j] = buckets[j], buckets[i]
	}
	// Move head of each bucket into buf, removing buckets that become empty.
	var i, j int
	for ; i < len(buf); i, j = i+1, (j+1)%len(buckets) {
		b := buckets[j]
		buf[i] = b[0]
		buckets[j] = b[1:]
		if len(b) == 1 {
			buckets = append(buckets[:j], buckets[j+1:]...)
		}
		if len(buckets) == 0 {
			break
		}
	}
	return i + 1
}



// lookup performs a network search for nodes close to the given target. It approaches the
// target by querying nodes that are closer to it on each iteration. The given target does
// not need to be an actual node identifier.
func (tab *Table) lookup(targetKey encPubkey, refreshIfEmpty bool) []*Node {
	//tab.log("lookup ", refreshIfEmpty)
	var (
		target         = ID(crypto.Keccak256Hash(targetKey[:]))
		asked          = make(map[ID]bool)
		seen           = make(map[ID]bool)
	//	reply          = make(chan []*Node, alpha)
		pendingQueries = 0
		result         *nodesByDistance
	)
	// don't query further if we hit ourself.
	// unlikely to happen often in practice.
	asked[tab.self().ID()] = true


	// generate initial result set
	result = tab.closest(target, bucketSize)

	if len(result.entries) == 0 && refreshIfEmpty {
		// make a table refresh and wait for it to finish
		tab.refresh()
		tab.refreshDone.Wait(true)
		// get the closest results
		result = tab.closest(target, bucketSize)
	}



	tab.lookupNode(&pendingQueries, result, asked, seen, targetKey)


	return result.entries
}

func (tab *Table) lookupNode(pendingQueries *int, result *nodesByDistance, asked map[ID]bool, seen map[ID]bool, targetKey encPubkey)  {

	// ask the alpha closest nodes that we haven't asked yet
	// when a node responds, ask another node if we can
	// the goal is to ask all the closest nodes with maximum of alpha pending requests

	// ask the alpha closest nodes that we haven't asked yet
	for i := 0; i < len(result.entries) && *pendingQueries < alpha; i++ {
	//	tab.log("lookupnode:", len(result.entries), *pendingQueries, len(asked), len(seen))
		n := result.entries[i]
		if !asked[n.ID()] {
			asked[n.ID()] = true
			*pendingQueries++

		//	tab.log("lookup-findnode ", targetKey)
			// go
			tab.findnode(n, targetKey, func(nodesByDist *nodesByDistance, err error) {
				if nodesByDist != nil {

					nodes := nodesByDist.entries
					for _, n := range nodes {
						if n != nil && !seen[n.ID()] {
							seen[n.ID()] = true
							result.push(n, bucketSize)
						}
					}
				}
				*pendingQueries--

				tab.lookupNode(pendingQueries, result, asked, seen, targetKey)
			})
		}
	}


}

func (tab *Table) findnode(n *Node, targetKey encPubkey, onResponse func(nodes *nodesByDistance, err error)) {

	//tab.log("findnode: ", n.Name())

	tab.net.findnode(n, targetKey, func(nodes *nodesByDistance, err error) {

		//tab.log("findnode respond ", nodes)
		fails := tab.db.FindFails(n)

		if err == errMsgTimeout {
			fails++
			tab.db.UpdateFindFails(n, fails)
			tab.log("Findnode failed", "id", n, "failcount", fails, "err", err)
			if fails >= maxFindnodeFailures {
				tab.log("Too many findnode failures, dropping", "id", n, "failcount", fails)
				tab.delete(n)
			}
		} else if fails > 0 {
			tab.db.UpdateFindFails(n, fails-1)
		}

		if nodes != nil {
			r := nodes.entries
			// Grab as many nodes as possible. Some of them might not be alive anymore, but we'll
			// just remove those again during revalidation.
			for _, n := range r {
				tab.addSeenNode(n)
			}
		}

		if onResponse != nil {
			onResponse(nodes, err)
		}

	})


}

func (tab *Table) refresh() {
	if tab.refreshDone.GetState(){
		tab.doRefresh()
	}
}

// loop schedules refresh, revalidate runs and coordinates shutdown.
func (tab *Table) loop() {
//	tab.log("table loop")

	// Start initial refresh.
	tab.doRefresh()

	tab.startRefreshing()

	tab.startRevalidateing()

}
func (tab *Table) SetOnline(online bool)  {

	if !online{
		tab.goOffline()
	} else {
		tab.goOnline()
	}

}
func (tab *Table) goOffline()  {

	for _, tr := range tab.tableRunners {
		tr.stop()
	}

	tab.tableRunners = nil

	tab.copyLiveNodes()

}

func (tab *Table) goOnline()  {

	for i := range tab.buckets {
		tab.buckets[i] = &bucket{}
	}

	tab.tableRunners = make([]*tableRunner, 0)

	tab.seedRand()

	tab.loadSeedNodes()

	tab.loop()
}

type tableRunner struct {
	*godes.Runner
	tab *Table
	stopped bool

}


func (tableRunner *tableRunner) stop()  {
	tableRunner.stopped = true
}

func (tab *Table) newTableRunner() *tableRunner {
	tr :=  &tableRunner{&godes.Runner{}, tab, false}
	tab.tableRunners = append(tab.tableRunners, tr)
	return tr
}

type refreshRunner struct {
	*tableRunner
}

func (r *refreshRunner) Run()  {
	refreshTime := refreshInterval.Seconds()
	tab := r.tab
	for{
		godes.Advance(refreshTime)

		if r.stopped {
			return
		}

		tab.doRefresh()
	}

}


func (tab *Table) startRefreshing()  {
	godes.AddRunner(&refreshRunner{tab.newTableRunner()})
}


type revalidateRunner struct {
	*tableRunner
}

func (r *revalidateRunner) Run()  {
	tab := r.tab
	for {

		t := tab.nextRevalidateTime().Seconds()

		godes.Advance(t)

		if r.stopped {
			return
		}
		//tab.log("table start revalidate ", t)
		tab.doRevalidate()
	}

}

func (tab *Table) startRevalidateing()  {
	godes.AddRunner(&revalidateRunner{tab.newTableRunner()})
}




// doRefresh performs a lookup for a random target to keep buckets
// full. seed nodes are inserted if the table is empty (initial
// bootstrap or discarded faulty peers).
func (tab *Table) doRefresh() {

	//tab.log("table do refresh ")
	tab.refreshDone.Set(false)

	tab.loadSeedNodes()

	// Run self lookup to discover new neighbor nodes.
	// We can only do this if we have a secp256k1 identity.
	tab.lookup(EncodePubKey(tab.self().publicKey), false)

	// The Kademlia paper specifies that the bucket refresh should
	// perform a lookup in the least recently used bucket. We cannot
	// adhere to this because the findnode target is a 512bit value
	// (not hash-sized) and it is not easily possible to generate a
	// sha3 preimage that falls into a chosen bucket.
	// We perform a few lookups with a random target instead.
	for i := 0; i < 3; i++ {
		var target encPubkey
		crand.Read(target[:])
		tab.lookup(target, false)
	}

	tab.refreshDone.Set(true)
}



func (tab *Table) copyLiveNodes() {
	tab.db.lastLiveNodes = tab.getLiveNodes(seedMaxAge)
}

func (tab *Table) loadSeedNodes() {
	nodes := tab.db.QuerySeeds(seedCount)
	nodes = append(nodes, tab.nursery...)
	for i := range nodes {
		seed := nodes[i]
		//tab.log("Load boostrap node: ", seed.Name())
		tab.addSeenNode(seed)
	}
}




// doRevalidate checks that the last node in a random bucket is still live
// and replaces or deletes the node if it isn't.
func (tab *Table) doRevalidate() {

	//tab.log("table do revalidate ")
	last, bi := tab.nodeToRevalidate()
	if last == nil {
		// No non-empty bucket found.
		return
	}

	// Ping the selected node and wait for a ping.
	//err := tab.net.ping(last)

	tab.self().sendPingPackage(last, func(m *Message, err error) {
	//	tab.log("ping respond, ", m)
		b := tab.buckets[bi]
		// if the response is received withing the timeout time
		if err == nil || err != errMsgTimeout {
			// The node responded, move it to the front.
			last.livenessChecks[tab.self()]++
			tab.log("Revalidated node", "b", bi, "id", last, "checks", last.livenessChecks[tab.self()])
			tab.bumpInBucket(b, last)
			return
		}
		// No reply received, pick a replacement or delete the node if there aren't
		// any replacements.
		if r := tab.replace(b, last); r != nil {
			tab.log("Replaced dead node", "b", bi, "id", last, "checks", last.livenessChecks[tab.self()], "r", r)
		} else {
			tab.log("Removed dead node", "b", bi, "id", last, "checks", last.livenessChecks[tab.self()])
		}

	})
}




// nodeToRevalidate returns the last node in a random, non-empty bucket.
func (tab *Table) nodeToRevalidate() (n *Node, bi int) {

	for _, bi = range tab.rand.Perm(len(tab.buckets)) {
		b := tab.buckets[bi]
		if len(b.entries) > 0 {
			last := b.entries[len(b.entries)-1]
			return last, bi
		}
	}
	return nil, 0
}

func (tab *Table) nextRevalidateTime() time.Duration {
	return time.Duration(tab.rand.Int63n(int64(revalidateInterval)))
}



func (tab *Table) Closest(pubKey encPubkey) *nodesByDistance {
	target := ID(crypto.Keccak256Hash(pubKey[:]))
	return  tab.closest(target, bucketSize)
}

// closest returns the n nodes in the table that are closest to the
// given id. The caller must hold tab.mutex.
func (tab *Table) closest(target ID, nresults int) *nodesByDistance {
	// This is a very wasteful way to find the closest nodes but
	// obviously correct. I believe that tree-based buckets would make
	// this easier to implement efficiently.
	close := &nodesByDistance{target: target}
	for _, b := range &tab.buckets {
		for _, n := range b.entries {
			if n.livenessChecks[tab.self()] > 0 {
				close.push(n, nresults)
			}
		}
	}
	return close
}

func (tab *Table) len() (n int) {
	for _, b := range &tab.buckets {
		n += len(b.entries)
	}
	return n
}

// bucket returns the bucket for the given node ID hash.
func (tab *Table) bucket(id ID) *bucket {
	d := LogDist(tab.self().ID(), id)
	if d <= bucketMinDistance {
		return tab.buckets[0]
	}
	return tab.buckets[d-bucketMinDistance-1]
}

// addSeenNode adds a node which may or may not be live to the end of a bucket. If the
// bucket has space available, adding the node succeeds immediately. Otherwise, the node is
// added to the replacements list.
func (tab *Table) addSeenNode(n *Node) {
	if n.ID() == tab.self().ID() {
		return
	}

	b := tab.bucket(n.ID())
	if contains(b.entries, n.ID()) {
		// Already in bucket, don't add.
		return
	}
	if len(b.entries) >= bucketSize {
		// Bucket full, maybe add as replacement.
		tab.addReplacement(b, n)
		return
	}

	tab.log("addSeenNode: ", n.Name())
	// Add to end of bucket:
	b.entries = append(b.entries, n)
	b.replacements = deleteNode(b.replacements, n)
	n.addedAt[tab.self()] = godes.GetSystemTime()
}

// addVerifiedNode adds a node whose existence has been verified recently to the front of a
// bucket. If the node is already in the bucket, it is moved to the front. If the bucket
// has no space, the node is added to the replacements list.
//
// There is an additional safety measure: if the table is still initializing the node
// is not added. This prevents an attack where the table could be filled by just sending
// sendPingPackage repeatedly.
//
// The caller must not hold tab.mutex.
func (tab *Table) addVerifiedNode(n *Node) {
	if n.ID() == tab.self().ID() {
		return
	}

	b := tab.bucket(n.ID())
	if tab.bumpInBucket(b, n) {
		// Already in bucket, moved to front.
		return
	}
	if len(b.entries) >= bucketSize {
		// Bucket full, maybe add as replacement.
		tab.addReplacement(b, n)
		return
	}
	// Add to front of bucket.
	b.entries, _ = pushNode(b.entries, n, bucketSize)
	b.replacements = deleteNode(b.replacements, n)
	n.addedAt[tab.self()] = godes.GetSystemTime()
}

// delete removes an entry from the node table. It is used to evacuate dead nodes.
func (tab *Table) delete(node *Node) {
	tab.deleteInBucket(tab.bucket(node.ID()), node)
}

func (tab *Table) addReplacement(b *bucket, n *Node) {
	for _, e := range b.replacements {
		if e.ID() == n.ID() {
			return // already in list
		}
	}

	tab.log("addReplacement: ", n.Name())
	var _ *Node
	b.replacements, _ = pushNode(b.replacements, n, maxReplacements)
}

// replace removes n from the replacement list and replaces 'last' with it if it is the
// last entry in the bucket. If 'last' isn't the last entry, it has either been replaced
// with someone else or became active.
func (tab *Table) replace(b *bucket, last *Node) *Node {
	if len(b.entries) == 0 || b.entries[len(b.entries)-1].ID() != last.ID() {
		// Entry has moved, don't replace it.
		return nil
	}
	// Still the last entry.
	if len(b.replacements) == 0 {
		tab.deleteInBucket(b, last)
		return nil
	}
	r := b.replacements[tab.rand.Intn(len(b.replacements))]
	b.replacements = deleteNode(b.replacements, r)
	b.entries[len(b.entries)-1] = r
	return r
}

// bumpInBucket moves the given node to the front of the bucket entry list
// if it is contained in that list.
func (tab *Table) bumpInBucket(b *bucket, n *Node) bool {
	for i := range b.entries {
		if b.entries[i].ID() == n.ID() {
			// Move it to the front.
			copy(b.entries[1:], b.entries[:i])
			b.entries[0] = n
			return true
		}
	}
	return false
}

func (tab *Table) deleteInBucket(b *bucket, n *Node) {
	b.entries = deleteNode(b.entries, n)
}

func contains(ns []*Node, id ID) bool {
	for _, n := range ns {
		if n.ID() == id {
			return true
		}
	}
	return false
}

// pushNode adds n to the front of list, keeping at most max items.
func pushNode(list []*Node, n *Node, max int) ([]*Node, *Node) {
	if len(list) < max {
		list = append(list, nil)
	}
	removed := list[len(list)-1]
	copy(list[1:], list)
	list[0] = n
	return list, removed
}

// deleteNode removes n from list.
func deleteNode(list []*Node, n *Node) []*Node {
	for i := range list {
		if list[i].ID() == n.ID() {
			return append(list[:i], list[i+1:]...)
		}
	}
	return list
}

// nodesByDistance is a list of nodes, ordered by
// distance to target.
type nodesByDistance struct {
	entries []*Node
	target  ID
}

func (n *nodesByDistance) String() string {
	return string(len(n.entries))
}
// push adds the given node to the list, keeping the total size below maxElems.
func (h *nodesByDistance) push(n *Node, maxElems int) {
	ix := sort.Search(len(h.entries), func(i int) bool {
		return DistCmp(h.target, h.entries[i].ID(), n.ID()) > 0
	})
	if len(h.entries) < maxElems {
		h.entries = append(h.entries, n)
	}
	if ix == len(h.entries) {
		// farther away than all nodes we already have.
		// if there was room for it, the node is now the last element.
	} else {
		// slide existing entries down to make room
		// this will overwrite the entry we just appended.
		copy(h.entries[ix+1:], h.entries[ix:])
		h.entries[ix] = n
	}
}

// DistCmp compares the distances a->target and b->target.
// Returns -1 if a is closer to target, 1 if b is closer to target
// and 0 if they are equal.
func DistCmp(target, a, b ID) int {
	for i := range target {
		da := a[i] ^ target[i]
		db := b[i] ^ target[i]
		if da > db {
			return 1
		} else if da < db {
			return -1
		}
	}
	return 0
}

// LogDist returns the logarithmic distance between a and b, log2(a ^ b).
func LogDist(a, b ID) int {
	lz := 0
	for i := range a {
		x := a[i] ^ b[i]
		if x == 0 {
			lz += 8
		} else {
			lz += bits.LeadingZeros8(x)
			break
		}
	}
	return len(a)*8 - lz
}



func (tab *Table) getLiveNodes(maxAge time.Duration) []*Node {
	nodes := make([]*Node, 0)
	now := godes.GetSystemTime()

	for _, b := range &tab.buckets {
		for _, n := range b.entries {

			if n.livenessChecks[tab.self()] > 0 &&
				now - n.addedAt[tab.self()] >= seedMinTableTime.Seconds()  &&
				now - tab.db.LastPongReceived(n) <= maxAge.Seconds(){
					nodes = append(nodes, n)
			}
		}
	}

	return nodes
}
