package main

import (
	"hash/fnv"
	"sort"
	"strconv"
	"sync"
)

// Ring is a consistent hash ring with virtual nodes.
type Ring struct {
	mu     sync.RWMutex      // lets many readers (Get) run at once, but only one writer (Add/Remove)
	vnodes int               // virtual nodes per real node
	hashes []uint64          // sorted positions on the ring
	owners map[uint64]string // position -> real node name
}

func NewRing(vnodes int) *Ring {
	return &Ring{vnodes: vnodes, owners: make(map[uint64]string)}
}

func hashKey(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	x := h.Sum64()
	x ^= x >> 33
	x *= 0xff51afd7ed558ccd
	x ^= x >> 33
	x *= 0xc4ceb9fe1a85ec53
	x ^= x >> 33
	return x
}

func (r *Ring) Add(node string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := 0; i < r.vnodes; i++ {
		hash := hashKey(node + "#" + strconv.Itoa(i))
		r.owners[hash] = node
		r.hashes = append(r.hashes, hash)
	}
	sort.Slice(r.hashes, func(a, b int) bool {
		return r.hashes[a] < r.hashes[b]
	})

}

func (r *Ring) Get(key string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.hashes) == 0 {
		return ""
	}
	h := hashKey(key)
	i := sort.Search(len(r.hashes), func(i int) bool { return r.hashes[i] >= h })
	if i == len(r.hashes) {
		i = 0
	}
	return r.owners[r.hashes[i]]
}

func (r *Ring) Remove(node string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := 0; i < r.vnodes; i++ {
		hash := hashKey(node + "#" + strconv.Itoa(i))
		delete(r.owners, hash)
	}
	r.hashes = r.hashes[:0]
	for hash := range r.owners {
		r.hashes = append(r.hashes, hash)
	}
	sort.Slice(r.hashes, func(a, b int) bool {
		return r.hashes[a] < r.hashes[b]
	})
}

// Nodes returns the real nodes currently on the ring, sorted.
func (r *Ring) Nodes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := make(map[string]bool)
	for _, owner := range r.owners {
		seen[owner] = true
	}
	nodes := []string{}
	for node := range seen {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)
	return nodes
}
