/*
Copyright 2022 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package helpers

import (
	"fmt"

	"github.com/onsi/gomega/format"
	configv1 "github.com/openshift/api/config/v1"
)

// clusterOperatorStatus is a helper struct to hold the status of a cluster operator.
// This allows us to work out which cluster operators failed the checks.
type clusterOperatorStatus struct {
	Name        string
	Available   bool
	Progressing bool
	Degraded    bool
}

// clusterOperatorConditions is a helper struct to hold the conditions of a named cluster operator.
type clusterOperatorConditions struct {
	Name       string
	Conditions []configv1.ClusterOperatorStatusCondition
}

// filteredClusterOperatorConditions is a helper struct to hold the conditions of named cluster operators
// that did not pass the test.
// We append a message to the top to warn users about failed cluster operators.
type filteredClusterOperatorConditions struct {
	Message          string
	ClusterOperators []clusterOperatorConditions
}

// formateClusterOperatorConditions formats the cluster operator conditions into a string.
// It filters any cluster operators that are available, not progressing and not degraded
// as these are the expected conditions.
func formatClusterOperatorsCondtions(in interface{}) (string, bool) {
	clusterOperators, ok := in.([]configv1.ClusterOperator)
	if !ok {
		return "", false
	}

	coConditions := []clusterOperatorConditions{}
	coNames := []string{}

	for _, co := range clusterOperators {
		coStatus := getClusterOperatorStatus(co)

		// Only list the cluster operators that are not available, progressing or degraded.
		// These are the ones we expect to fail the test.
		if coStatus.Available && !coStatus.Progressing && !coStatus.Degraded {
			continue
		}

		// output the full set of conditions so that the test has the full context.
		coConditions = append(coConditions, clusterOperatorConditions{
			Name:       co.Name,
			Conditions: co.Status.Conditions,
		})

		coNames = append(coNames, co.Name)
	}

	out := filteredClusterOperatorConditions{
		Message:          fmt.Sprintf("Cluster operators %s are either not available, are progressing or are degraded.", coNames),
		ClusterOperators: coConditions,
	}

	return format.Object(out, 1), true
}

// getClusterOperatorStatus returns the status of a cluster operator in terms of it's
// available, progressing and degraded conditions.
func getClusterOperatorStatus(co configv1.ClusterOperator) clusterOperatorStatus {
	coStatus := clusterOperatorStatus{
		Name: co.Name,
	}

	for _, condition := range co.Status.Conditions {
		switch condition.Type {
		case configv1.OperatorAvailable:
			coStatus.Available = condition.Status == configv1.ConditionTrue
		case configv1.OperatorProgressing:
			coStatus.Progressing = condition.Status == configv1.ConditionTrue
		case configv1.OperatorDegraded:
			coStatus.Degraded = condition.Status == configv1.ConditionTrue
		case configv1.OperatorUpgradeable, configv1.EvaluationConditionsDetected, configv1.RetrievedUpdates:
			continue
		}
	}

	return coStatus
}
