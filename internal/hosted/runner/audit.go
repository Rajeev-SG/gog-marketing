package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"sync/atomic"
)

// Counter records finished executions. It is an injected seam so tests can
// verify counting behavior (including rejected requests) without faking
// positive responses in production.
type Counter interface {
	// Record counts one finished execution for the operation/outcome pair.
	Record(operation, outcome string)

	// Total reports the cumulative executed request count.
	Total() int64
}

// NewDefaultCounter returns the production counter.
func NewDefaultCounter() Counter {
	return &defaultCounter{}
}

type defaultCounter struct {
	total atomic.Int64
}

func (c *defaultCounter) Record(string, string) {
	c.total.Add(1)
}

func (c *defaultCounter) Total() int64 {
	return c.total.Load()
}

// countingCounter is a test-visible counter that also keeps per-outcome
// buckets so negative tests can assert rejection counts precisely.
type countingCounter struct {
	mu    sync.Mutex
	byKey map[string]int64
}

func newCountingCounter() *countingCounter {
	return &countingCounter{byKey: make(map[string]int64)}
}

func (c *countingCounter) Record(operation, outcome string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.byKey[operation+"\x00"+outcome]++
}

func (c *countingCounter) Total() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	total := int64(0)
	for _, count := range c.byKey {
		total += count
	}

	return total
}

func (c *countingCounter) count(operation, outcome string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.byKey[operation+"\x00"+outcome]
}

// hashContext returns a small irreversible hash for audit fields. It is not
// reversible to the tenant or connection identifier within the audit line.
func hashContext(value string) string {
	digest := sha256.Sum256([]byte(value))

	return hex.EncodeToString(digest[:])[:16]
}
