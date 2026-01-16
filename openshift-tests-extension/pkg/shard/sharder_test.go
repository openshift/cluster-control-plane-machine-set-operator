package shard_test

import (
	"fmt"
	"testing"

	"github.com/openshift/cluster-control-plane-machine-set-operator/openshift-tests-extension/pkg/shard"
)

func TestHashSharder_Distribution(t *testing.T) {
	sharder := shard.NewHashSharder()

	// Generate test names
	testNames := make([]string, 100)
	for i := 0; i < 100; i++ {
		testNames[i] = fmt.Sprintf("test-case-%d", i)
	}

	// Test with 4 shards
	shardCount := 4
	distribution := make(map[int]int)

	for _, name := range testNames {
		for shardIndex := 1; shardIndex <= shardCount; shardIndex++ {
			if sharder.ShouldRun(name, shardIndex, shardCount) {
				distribution[shardIndex]++
			}
		}
	}

	// Verify each test assigned to exactly one shard
	total := 0
	for _, count := range distribution {
		total += count
	}
	if total != len(testNames) {
		t.Errorf("Expected %d tests total, got %d", len(testNames), total)
	}

	// Verify reasonable balance (within 30% of average)
	avgPerShard := len(testNames) / shardCount
	tolerance := float64(avgPerShard) * 0.3

	for shard, count := range distribution {
		diff := float64(count - avgPerShard)
		if diff < 0 {
			diff = -diff
		}
		if diff > tolerance {
			t.Errorf("Shard %d has %d tests, expected ~%d (tolerance: %.0f)",
				shard, count, avgPerShard, tolerance)
		}
	}
}

func TestHashSharder_Determinism(t *testing.T) {
	sharder := shard.NewHashSharder()
	testName := "test-determinism"

	// Same test should always go to same shard
	firstResult := sharder.ShouldRun(testName, 2, 5)
	for i := 0; i < 100; i++ {
		if sharder.ShouldRun(testName, 2, 5) != firstResult {
			t.Error("Sharder is not deterministic")
		}
	}
}

func TestHashSharder_NoOverlap(t *testing.T) {
	sharder := shard.NewHashSharder()
	testNames := []string{
		"test-case-1",
		"test-case-2",
		"test-case-3",
		"test-case-4",
		"test-case-5",
	}

	shardCount := 3

	for _, name := range testNames {
		assignedCount := 0
		for shardIndex := 1; shardIndex <= shardCount; shardIndex++ {
			if sharder.ShouldRun(name, shardIndex, shardCount) {
				assignedCount++
			}
		}
		if assignedCount != 1 {
			t.Errorf("Test %s assigned to %d shards, expected exactly 1", name, assignedCount)
		}
	}
}

func TestHashSharder_DisabledSharding(t *testing.T) {
	sharder := shard.NewHashSharder()
	testName := "test-disabled"

	// When sharding is disabled (shardTotal=0), all tests should run
	if !sharder.ShouldRun(testName, 0, 0) {
		t.Error("Test should run when sharding is disabled")
	}

	// Invalid config should default to running all tests
	if !sharder.ShouldRun(testName, 5, 3) {
		t.Error("Test should run with invalid config (index > total)")
	}

	if !sharder.ShouldRun(testName, 0, 3) {
		t.Error("Test should run with invalid config (index = 0)")
	}
}

func TestValidateShardConfig(t *testing.T) {
	tests := []struct {
		name        string
		shardIndex  int
		shardTotal  int
		expectError bool
	}{
		{"disabled", 0, 0, false},
		{"valid 1/4", 1, 4, false},
		{"valid 4/4", 4, 4, false},
		{"index too high", 5, 4, true},
		{"index zero with total", 0, 4, true},
		{"negative index", -1, 4, true},
		{"negative total", 1, -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := shard.ValidateShardConfig(tt.shardIndex, tt.shardTotal)
			if (err != nil) != tt.expectError {
				t.Errorf("ValidateShardConfig() error = %v, expectError %v", err, tt.expectError)
			}
		})
	}
}

func TestHashSharder_Name(t *testing.T) {
	sharder := shard.NewHashSharder()
	if sharder.Name() != "hash" {
		t.Errorf("Expected sharder name to be 'hash', got '%s'", sharder.Name())
	}
}

func TestHashSharder_LargeScale(t *testing.T) {
	sharder := shard.NewHashSharder()

	// Test with a larger number of tests and shards
	testCount := 1000
	shardCount := 10

	testNames := make([]string, testCount)
	for i := 0; i < testCount; i++ {
		testNames[i] = fmt.Sprintf("large-test-%d", i)
	}

	distribution := make(map[int]int)

	for _, name := range testNames {
		for shardIndex := 1; shardIndex <= shardCount; shardIndex++ {
			if sharder.ShouldRun(name, shardIndex, shardCount) {
				distribution[shardIndex]++
			}
		}
	}

	// Verify each test assigned to exactly one shard
	total := 0
	for _, count := range distribution {
		total += count
	}
	if total != testCount {
		t.Errorf("Expected %d tests total, got %d", testCount, total)
	}

	// Verify reasonable balance (within 30% of average for large scale)
	avgPerShard := testCount / shardCount
	tolerance := float64(avgPerShard) * 0.3

	for shard, count := range distribution {
		diff := float64(count - avgPerShard)
		if diff < 0 {
			diff = -diff
		}
		if diff > tolerance {
			t.Errorf("Shard %d has %d tests, expected ~%d (tolerance: %.0f)",
				shard, count, avgPerShard, tolerance)
		}
	}
}
