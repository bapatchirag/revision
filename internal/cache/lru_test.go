package cache

import "testing"

// sizeOfString weighs a string value by its length, so the byte-budget tests can
// reason in characters.
func sizeOfString(s string) int { return len(s) }

func TestLRUEvictsByEntryCount(t *testing.T) {
	c := New[string, string](2, 0, sizeOfString)
	c.Put("a", "1")
	c.Put("b", "2")
	c.Put("c", "3")

	if c.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", c.Len())
	}
	if _, ok := c.Get("a"); ok {
		t.Error("a should have been evicted as the least recently used entry")
	}
	for _, k := range []string{"b", "c"} {
		if _, ok := c.Get(k); !ok {
			t.Errorf("%s should still be cached", k)
		}
	}
}

func TestLRUEvictsByByteBudget(t *testing.T) {
	c := New[string, string](0, 10, sizeOfString)
	c.Put("a", "12345")
	c.Put("b", "12345")
	if c.Bytes() != 10 {
		t.Fatalf("Bytes() = %d, want 10", c.Bytes())
	}

	// The budget is full, so storing another entry evicts the oldest.
	c.Put("c", "123")
	if _, ok := c.Get("a"); ok {
		t.Error("a should have been evicted to make room within the byte budget")
	}
	if c.Bytes() != 8 {
		t.Errorf("Bytes() = %d, want 8", c.Bytes())
	}
}

// TestLRUDropsOversizedValue pins the deliberate behaviour that a value heavier
// than the whole budget is not cached at all, rather than emptying the cache to
// make room for itself.
func TestLRUDropsOversizedValue(t *testing.T) {
	c := New[string, string](0, 4, sizeOfString)
	c.Put("small", "ab")
	c.Put("huge", "abcdefghij")

	if _, ok := c.Get("huge"); ok {
		t.Error("an entry larger than the budget should not be retained")
	}
	if c.Len() != 0 {
		t.Errorf("Len() = %d, want 0; the oversized entry displaced the rest", c.Len())
	}
}

func TestLRUGetRenewsRecency(t *testing.T) {
	c := New[string, string](2, 0, sizeOfString)
	c.Put("a", "1")
	c.Put("b", "2")
	c.Get("a") // a is now the most recently used, so b is next to go
	c.Put("c", "3")

	if _, ok := c.Get("a"); !ok {
		t.Error("a was just read and should have survived")
	}
	if _, ok := c.Get("b"); ok {
		t.Error("b was the least recently used and should have been evicted")
	}
}

func TestLRUPutReplacesInPlace(t *testing.T) {
	c := New[string, string](2, 0, sizeOfString)
	c.Put("a", "12345")
	c.Put("a", "1")

	if c.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", c.Len())
	}
	if c.Bytes() != 1 {
		t.Errorf("Bytes() = %d, want 1; the replaced value's weight was not discounted", c.Bytes())
	}
	if v, _ := c.Get("a"); v != "1" {
		t.Errorf("Get(a) = %q, want %q", v, "1")
	}
}

func TestLRUDelete(t *testing.T) {
	c := New[string, string](0, 0, sizeOfString)
	c.Put("a", "12")
	c.Delete("a")
	c.Delete("missing") // deleting what is not there is a no-op

	if c.Len() != 0 || c.Bytes() != 0 {
		t.Errorf("after Delete: Len() = %d, Bytes() = %d, want 0 and 0", c.Len(), c.Bytes())
	}
}

func TestLRUDeleteFunc(t *testing.T) {
	c := New[string, string](0, 0, sizeOfString)
	c.Put("keep", "1")
	c.Put("drop", "22")
	c.Put("drop-too", "333")

	c.DeleteFunc(func(k, _ string) bool { return k != "keep" })

	if c.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", c.Len())
	}
	if _, ok := c.Get("keep"); !ok {
		t.Error("keep should have survived DeleteFunc")
	}
	if c.Bytes() != 1 {
		t.Errorf("Bytes() = %d, want 1", c.Bytes())
	}
}

func TestLRUPurge(t *testing.T) {
	c := New[string, string](0, 0, sizeOfString)
	c.Put("a", "1")
	c.Put("b", "2")
	c.Purge()

	if c.Len() != 0 || c.Bytes() != 0 {
		t.Errorf("after Purge: Len() = %d, Bytes() = %d, want 0 and 0", c.Len(), c.Bytes())
	}
	// The cache stays usable afterwards.
	c.Put("c", "3")
	if _, ok := c.Get("c"); !ok {
		t.Error("cache should still accept entries after Purge")
	}
}

// TestLRUWithoutSizeOf covers the count-only configuration, where values weigh
// nothing and the byte budget never applies.
func TestLRUWithoutSizeOf(t *testing.T) {
	c := New[int, int](2, 8, nil)
	c.Put(1, 1)
	c.Put(2, 2)
	c.Put(3, 3)

	if c.Bytes() != 0 {
		t.Errorf("Bytes() = %d, want 0 without a sizeOf", c.Bytes())
	}
	if c.Len() != 2 {
		t.Errorf("Len() = %d, want 2", c.Len())
	}
}
