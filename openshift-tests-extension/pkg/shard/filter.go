package shard

import (
	"github.com/openshift-eng/openshift-tests-extension/pkg/extension/extensiontests"
)

// SelectByShard returns a SelectFunction that filters tests by shard
func SelectByShard(sharder Sharder, shardIndex, shardTotal int) extensiontests.SelectFunction {
	return func(spec *extensiontests.ExtensionTestSpec) bool {
		return sharder.ShouldRun(spec.Name, shardIndex, shardTotal)
	}
}

// FilterSpecsByShard applies sharding to a spec list
func FilterSpecsByShard(specs extensiontests.ExtensionTestSpecs, shardIndex, shardTotal int) extensiontests.ExtensionTestSpecs {
	if shardTotal == 0 || shardIndex == 0 {
		return specs // No sharding
	}

	sharder := NewHashSharder()
	return specs.Select(SelectByShard(sharder, shardIndex, shardTotal))
}
