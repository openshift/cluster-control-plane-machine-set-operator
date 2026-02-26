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
	"time"

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

// formatClusterOperatorsConditions formats the cluster operator conditions into a string.
// It filters any cluster operators that are available, not progressing and not degraded
// as these are the expected conditions. Handles both *configv1.ClusterOperatorList (as
// returned by Eventually(komega.ObjectList(...))) and []configv1.ClusterOperator.
func formatClusterOperatorsConditions(in interface{}) (string, bool) {
	var clusterOperators []configv1.ClusterOperator
	switch v := in.(type) {
	case *configv1.ClusterOperatorList:
		clusterOperators = v.Items
	case []configv1.ClusterOperator:
		clusterOperators = v
	default:
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

	var msg string
	if len(coConditions) == 0 {
		// No COs failed the availability/progressing/degraded filter; failure may be due to
		// conditions not being stable long enough. Show all COs so the user can see current state.
		msg = "Some cluster operators did not met the required conditions stability criteria (i.e. conditions not stable for the required minimumAvailability duration)."

		// Show all COs so the user can see last transition times for the conditions of each CO.
		for _, co := range clusterOperators {
			coConditions = append(coConditions, clusterOperatorConditions{Name: co.Name, Conditions: co.Status.Conditions})
		}
	} else {
		msg = fmt.Sprintf("Cluster operators %s are either not available, are progressing or are degraded.", coNames)
	}

	out := filteredClusterOperatorConditions{
		Message:          msg,
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

	// Operators that transitioned more recently than the stabilisationWindow are considered not yet stabilised.
	stabilisationWindow := 1 * time.Minute

	for _, condition := range co.Status.Conditions {
		switch condition.Type {
		case configv1.OperatorAvailable:
			// We consider the operator to be available if it is True and has been stable for the stabilisationWindow.
			coStatus.Available = condition.Status == configv1.ConditionTrue && time.Now().Add(-stabilisationWindow).After(condition.LastTransitionTime.Time)
		case configv1.OperatorProgressing:
			// We consider the operator to be progressing if it is True or transitioned within the stabilisationWindow.
			coStatus.Progressing = condition.Status == configv1.ConditionTrue || time.Now().Add(-stabilisationWindow).Before(condition.LastTransitionTime.Time)
		case configv1.OperatorDegraded:
			// We consider the operator to be degraded if it is True or transitioned within the stabilisationWindow.
			coStatus.Degraded = condition.Status == configv1.ConditionTrue || time.Now().Add(-stabilisationWindow).Before(condition.LastTransitionTime.Time)
		case configv1.OperatorUpgradeable, configv1.EvaluationConditionsDetected, configv1.RetrievedUpdates:
			continue
		}
	}

	return coStatus
}
