package shard

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// Sharder defines the interface for test sharding strategies
type Sharder interface {
	// ShouldRun determines if a test should run in this shard
	ShouldRun(testName string, shardIndex, shardTotal int) bool
	Name() string
}

// HashSharder implements hash-based test distribution
type HashSharder struct{}

func NewHashSharder() *HashSharder {
	return &HashSharder{}
}

func (h *HashSharder) Name() string {
	return "hash"
}

// ShouldRun uses SHA-256 hashing to deterministically assign tests to shards
// Algorithm matches openshift/origin implementation for consistency
func (h *HashSharder) ShouldRun(testName string, shardIndex, shardTotal int) bool {
	if shardTotal <= 0 || shardIndex <= 0 || shardIndex > shardTotal {
		return true // No sharding or invalid config
	}

	// Hash the test name
	sum := sha256.Sum256([]byte(testName))

	// Convert first 4 bytes to uint32
	val := binary.BigEndian.Uint32(sum[:4])

	// Assign to shard (1-indexed like origin)
	assignedShard := (int(val) % shardTotal) + 1

	return assignedShard == shardIndex
}

// ValidateShardConfig validates shard configuration
func ValidateShardConfig(shardIndex, shardTotal int) error {
	if shardTotal < 0 || shardIndex < 0 {
		return fmt.Errorf("shard-index and shard-total must be non-negative")
	}
	if shardTotal > 0 && shardIndex == 0 {
		return fmt.Errorf("shard-index must be >= 1 when shard-total is set")
	}
	if shardIndex > shardTotal {
		return fmt.Errorf("shard-index (%d) cannot exceed shard-total (%d)", shardIndex, shardTotal)
	}
	return nil
}
