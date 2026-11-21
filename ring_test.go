package main

import (
	"strconv"
	"testing"
)

// newTestRing builds a ring with the given nodes. Shared setup so each test stays short.
func newTestRing(vnodes int, nodes ...string) *Ring {
	r := NewRing(vnodes)
	for _, n := range nodes {
		r.Add(n)
	}
	return r
}

// TestRingEmpty: an empty ring must return "" instead of panicking.
func TestRingEmpty(t *testing.T) {
	r := NewRing(150)
	if got := r.Get("anything"); got != "" {
		t.Fatalf("empty ring: Get = %q, want \"\"", got)
	}
}

// TestRingDeterministic: the same key always maps to the same node.
func TestRingDeterministic(t *testing.T) {
	r := newTestRing(150, "kv1", "kv2", "kv3")
	for i := 0; i < 1000; i++ {
		first := r.Get("key_" + strconv.Itoa(i))
		second := r.Get("key_" + strconv.Itoa(i))
		if first != second {
			t.Fatalf("key %s: Get = %q, want %q", "key_"+strconv.Itoa(i), second, first)
		}
	}
}

// TestRingDistribution: with 150 vnodes each node gets roughly 1/3 of keys.
func TestRingDistribution(t *testing.T) {
	const numKeys = 100000
	r := newTestRing(150, "kv1", "kv2", "kv3")
	counts := map[string]int{}
	for i := 0; i < numKeys; i++ {
		counts[r.Get("key_"+strconv.Itoa(i))]++
	}
	for node, c := range counts {
		share := float64(c) / numKeys
		t.Logf("%s: %.1f%%", node, share*100)
		if share < 0.28 || share > 0.38 {
			t.Errorf("%s: got %.1f%%, want ~33%%", node, share*100)
		}
	}

}

// TestRingRemoveMovesOnlyThatNode: removing kv2 must not move kv1's or kv3's keys.
func TestRingRemoveMovesOnlyThatNode(t *testing.T) {
	r := newTestRing(150, "kv1", "kv2", "kv3")
	before := map[string]string{} // key -> owner before removal
	for i := 0; i < 10000; i++ {
		before["key_"+strconv.Itoa(i)] = r.Get("key_" + strconv.Itoa(i))
	}
	r.Remove("kv2")
	for key, old := range before {
		now := r.Get(key)
		if now == "kv2" {
			t.Errorf("%s: got %q, want != \"kv2\"", key, now)
		}
		if old != "kv2" && now != old {
			t.Errorf("%s: got %q, want %q", key, now, old)
		}
	}
}

// TestRingAddMovesAboutOneQuarter: going 3 -> 4 nodes should move about 1/4 of keys.
func TestRingAddMovesAboutOneQuarter(t *testing.T) {
	const numKeys = 10000
	r := newTestRing(150, "kv1", "kv2", "kv3")
	before := map[string]string{}
	for i := 0; i < numKeys; i++ {
		before["key_"+strconv.Itoa(i)] = r.Get("key_" + strconv.Itoa(i))
	}
	r.Add("kv4")
	moved := 0
	for key, old := range before {
		now := r.Get(key)
		if now != old {
			moved++
			if now != "kv4" {
				t.Errorf("%s: got %q, want \"kv4\"", key, now)
			}
		}
	}
	fraction := float64(moved) / numKeys
	t.Logf("moved fraction: %.2f%%", fraction*100)
	if fraction < 0.20 || fraction > 0.30 {
		t.Errorf("moved fraction: got %.2f%%, want ~25%%", fraction*100)
	}
}
