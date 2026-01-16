package flags

import (
	"fmt"

	"github.com/spf13/pflag"
)

// ShardFlags contains sharding configuration
type ShardFlags struct {
	ShardIndex int
	ShardTotal int
}

func NewShardFlags() *ShardFlags {
	return &ShardFlags{
		ShardIndex: 0,
		ShardTotal: 0,
	}
}

func (f *ShardFlags) BindFlags(fs *pflag.FlagSet) {
	fs.IntVar(&f.ShardIndex,
		"shard-index",
		f.ShardIndex,
		"When tests are sharded across instances, which shard this is (1-indexed, 0=disabled)",
	)
	fs.IntVar(&f.ShardTotal,
		"shard-total",
		f.ShardTotal,
		"Total number of shards (0=disabled)",
	)
}

func (f *ShardFlags) Validate() error {
	if f.ShardTotal < 0 || f.ShardIndex < 0 {
		return fmt.Errorf("shard-index and shard-total must be non-negative")
	}
	if f.ShardTotal > 0 && f.ShardIndex == 0 {
		return fmt.Errorf("shard-index must be >= 1 when shard-total is set")
	}
	if f.ShardIndex > f.ShardTotal {
		return fmt.Errorf("shard-index (%d) cannot exceed shard-total (%d)", f.ShardIndex, f.ShardTotal)
	}
	return nil
}

func (f *ShardFlags) IsEnabled() bool {
	return f.ShardTotal > 0 && f.ShardIndex > 0
}
