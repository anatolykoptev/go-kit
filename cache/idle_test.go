package cache

import (
	"testing"
	"time"
)

func TestIdleTTL_EvictsUnusedEntries(t *testing.T) {
	idleTTL := 1 * time.Second
	c := New(Config{
		L1MaxItems: 100,
		L1TTL:      1 * time.Hour, // far-away absolute expiry
		IdleTTL:    idleTTL,
	})
	defer c.Close()

	c.Set(nil, "hot", []byte("v1"))
	c.Set(nil, "cold", []byte("v2"))

	// Keep "hot" accessed; leave "cold" untouched.
	time.Sleep(idleTTL / 2)
	c.Get(nil, "hot")

	// Wait long enough that "cold" is idle-expired, but "hot" is still fresh.
	time.Sleep(idleTTL*3/4 + idleTTL/10)

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
