package cache

import (
	"testing"
	"time"
)

func TestIdleTTL_EvictsUnusedEntries(t *testing.T) {
	c := New(Config{
		L1MaxItems: 100,
		L1TTL:      1 * time.Hour, // far-away absolute expiry
		IdleTTL:    50 * time.Millisecond,
	})
	defer c.Close()
	c.Set(nil, "hot", []byte("v1"))
	c.Set(nil, "cold", []byte("v2"))
	// Keep "hot" accessed; leave "cold" untouched.
	time.Sleep(30 * time.Millisecond)
	c.Get(nil, "hot")
	time.Sleep(40 * time.Millisecond) // total 70ms — "cold" idle for 70, "hot" for 40
	// cold should now be idle-expired, hot should survive.
	_, coldOK := c.Get(nil, "cold")
	if coldOK {
		t.Error("cold entry must be idle-expired")
	}
	_, hotOK := c.Get(nil, "hot")
	if !hotOK {
		t.Error("hot entry (recently accessed) must survive")
	}
}

func TestIdleTTL_ZeroDisables(t *testing.T) {
	c := New(Config{L1MaxItems: 100, L1TTL: time.Hour, IdleTTL: 0})
	defer c.Close()
	c.Set(nil, "k", []byte("v"))
	time.Sleep(50 * time.Millisecond)
	// No idle expiry — entry must survive.
	if _, ok := c.Get(nil, "k"); !ok {
		t.Error("entry must survive when IdleTTL == 0")
	}
}
