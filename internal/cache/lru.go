// Package cache provides small, domain-agnostic in-memory caches. It knows
// nothing of Subversion or the terminal UI: callers supply the key, the value
// and how to weigh it.
package cache

import "container/list"

// LRU is a least-recently-used cache bounded by an entry count and a byte
// budget. It is not safe for concurrent use: it is meant to be owned by a single
// goroutine, so callers that need one from several must guard it themselves.
type LRU[K comparable, V any] struct {
	maxEntries int
	maxBytes   int
	sizeOf     func(V) int
	order      *list.List // front = most recently used
	items      map[K]*list.Element
	bytes      int
}

// node is one cached value together with the key it is stored under (so
// eviction can drop the map entry) and the size it was weighed at (so the
// running byte total stays correct when it is replaced or removed).
type node[K comparable, V any] struct {
	key   K
	value V
	size  int
}

// New returns a cache bounded by maxEntries and maxBytes; a non-positive bound
// is unlimited. sizeOf weighs a value against the byte budget, and a nil sizeOf
// leaves the entry count as the only bound. A value heavier than the whole
// budget is evicted as soon as it is stored, so one outsized entry can never
// displace the rest of the cache.
func New[K comparable, V any](maxEntries, maxBytes int, sizeOf func(V) int) *LRU[K, V] {
	return &LRU[K, V]{
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
		sizeOf:     sizeOf,
		order:      list.New(),
		items:      map[K]*list.Element{},
	}
}

// Get returns the value stored under k, marking it most recently used.
func (c *LRU[K, V]) Get(k K) (V, bool) {
	el, ok := c.items[k]
	if !ok {
		var zero V
		return zero, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*node[K, V]).value, true
}

// Put stores v under k as the most recently used entry, replacing whatever was
// there, then evicts from the least recently used end until both bounds hold.
func (c *LRU[K, V]) Put(k K, v V) {
	size := 0
	if c.sizeOf != nil {
		size = c.sizeOf(v)
	}
	if el, ok := c.items[k]; ok {
		n := el.Value.(*node[K, V])
		c.bytes += size - n.size
		n.value, n.size = v, size
		c.order.MoveToFront(el)
	} else {
		c.items[k] = c.order.PushFront(&node[K, V]{key: k, value: v, size: size})
		c.bytes += size
	}
	c.evict()
}

// Delete removes k if it is present.
func (c *LRU[K, V]) Delete(k K) {
	if el, ok := c.items[k]; ok {
		c.remove(el)
	}
}

// DeleteFunc removes every entry drop reports true for, leaving the recency
// order of the survivors untouched.
func (c *LRU[K, V]) DeleteFunc(drop func(K, V) bool) {
	for el := c.order.Front(); el != nil; {
		next := el.Next()
		n := el.Value.(*node[K, V])
		if drop(n.key, n.value) {
			c.remove(el)
		}
		el = next
	}
}

// Purge empties the cache.
func (c *LRU[K, V]) Purge() {
	c.order.Init()
	c.items = map[K]*list.Element{}
	c.bytes = 0
}

// Len returns the number of entries held.
func (c *LRU[K, V]) Len() int { return c.order.Len() }

// Bytes returns the total weight of the entries held, as measured by sizeOf.
func (c *LRU[K, V]) Bytes() int { return c.bytes }

// evict drops the least recently used entries until both bounds are satisfied.
func (c *LRU[K, V]) evict() {
	for c.order.Len() > 0 {
		overCount := c.maxEntries > 0 && c.order.Len() > c.maxEntries
		overBytes := c.maxBytes > 0 && c.bytes > c.maxBytes
		if !overCount && !overBytes {
			return
		}
		c.remove(c.order.Back())
	}
}

// remove unlinks el from both the recency order and the index, and discounts
// its weight.
func (c *LRU[K, V]) remove(el *list.Element) {
	n := el.Value.(*node[K, V])
	c.order.Remove(el)
	delete(c.items, n.key)
	c.bytes -= n.size
}
