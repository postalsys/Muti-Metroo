// Package agent provides the relay-table primitive used by the TCP, UDP,
// and ICMP transit handlers. All three protocols only need
// (upstream peer, upstream stream id, downstream peer, downstream stream id)
// to forward frames in either direction, so the relay-table type and the
// indexing logic are shared.
package agent

import (
	"sync"

	"github.com/postalsys/muti-metroo/internal/identity"
)

// relayEntry tracks a stream/association/session being relayed through this
// agent (transit). The same struct shape is used for TCP streams, UDP
// associations, and ICMP sessions; lifecycle management is identical.
//
// Entries are immutable once inserted into a relayTable: callers may safely
// dereference fields after a Lookup* method returns even if a concurrent
// goroutine deletes the entry, because pointer values returned to callers
// outlive the table's index entries.
type relayEntry struct {
	UpstreamPeer   identity.AgentID
	UpstreamID     uint64 // ID space of the upstream peer connection
	DownstreamPeer identity.AgentID
	DownstreamID   uint64 // ID space of the downstream peer connection (allocated locally)
}

// relayTable is a thread-safe bidirectional index of relay entries keyed
// by both upstream and downstream stream IDs. It is used by the TCP, UDP,
// and ICMP relay handlers, which all share the same shape.
//
// Invariant: every entry is always indexed under BOTH byUpstream and
// byDownstream. All mutators preserve this; future mutators must too.
// DeleteByPeer relies on it to walk only one map.
type relayTable struct {
	mu           sync.RWMutex
	byUpstream   map[uint64]*relayEntry
	byDownstream map[uint64]*relayEntry
}

// newRelayTable returns an empty relay table ready for use.
func newRelayTable() *relayTable {
	return &relayTable{
		byUpstream:   make(map[uint64]*relayEntry),
		byDownstream: make(map[uint64]*relayEntry),
	}
}

// Insert adds an entry under both upstream and downstream keys.
func (r *relayTable) Insert(e *relayEntry) {
	r.mu.Lock()
	r.byUpstream[e.UpstreamID] = e
	r.byDownstream[e.DownstreamID] = e
	r.mu.Unlock()
}

// Delete removes the entry from both indices. Idempotent.
func (r *relayTable) Delete(e *relayEntry) {
	r.mu.Lock()
	delete(r.byUpstream, e.UpstreamID)
	delete(r.byDownstream, e.DownstreamID)
	r.mu.Unlock()
}

// LookupBoth returns the entries (if any) where streamID matches the
// upstream and downstream IDs respectively. The streamID number space is
// per-peer-connection so the two indices can be hit by different entries
// using the same numeric value; callers disambiguate by checking the
// frame's source peer against UpstreamPeer / DownstreamPeer.
func (r *relayTable) LookupBoth(streamID uint64) (up, down *relayEntry) {
	r.mu.RLock()
	up = r.byUpstream[streamID]
	down = r.byDownstream[streamID]
	r.mu.RUnlock()
	return up, down
}

// LookupDownstream returns the entry whose DownstreamID == streamID, or nil.
// Used by ACK handlers, where the response always comes back over the
// downstream peer connection.
func (r *relayTable) LookupDownstream(streamID uint64) *relayEntry {
	r.mu.RLock()
	e := r.byDownstream[streamID]
	r.mu.RUnlock()
	return e
}

// PopDownstreamFromPeer atomically looks up an entry by DownstreamID, checks
// that the downstream peer matches `peer`, and removes it from both indices
// if so. Returns nil if no matching entry exists. Used by *_OPEN_ERR handlers.
func (r *relayTable) PopDownstreamFromPeer(streamID uint64, peer identity.AgentID) *relayEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.byDownstream[streamID]
	if e == nil || e.DownstreamPeer != peer {
		return nil
	}
	delete(r.byUpstream, e.UpstreamID)
	delete(r.byDownstream, e.DownstreamID)
	return e
}

// PopMatchingPeer atomically looks up an entry whose direction matches the
// frame's source peer, removes it from both indices, and returns it. The
// fromUpstream return flag is true when streamID matched on the upstream
// side (i.e., the close/reset originated from the upstream peer), false
// when it matched on the downstream side. Returns (nil, false) if no
// entry's peer matches.
func (r *relayTable) PopMatchingPeer(streamID uint64, peer identity.AgentID) (entry *relayEntry, fromUpstream bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if up := r.byUpstream[streamID]; up != nil && up.UpstreamPeer == peer {
		delete(r.byUpstream, up.UpstreamID)
		delete(r.byDownstream, up.DownstreamID)
		return up, true
	}
	if down := r.byDownstream[streamID]; down != nil && down.DownstreamPeer == peer {
		delete(r.byUpstream, down.UpstreamID)
		delete(r.byDownstream, down.DownstreamID)
		return down, false
	}
	return nil, false
}

// DeleteByPeer removes every entry where either the upstream or downstream
// peer is `peer`. Returns the number of entries removed. Used during peer
// disconnect cleanup.
func (r *relayTable) DeleteByPeer(peer identity.AgentID) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int
	for id, e := range r.byUpstream {
		if e.UpstreamPeer == peer || e.DownstreamPeer == peer {
			delete(r.byUpstream, id)
			delete(r.byDownstream, e.DownstreamID)
			n++
		}
	}
	return n
}
