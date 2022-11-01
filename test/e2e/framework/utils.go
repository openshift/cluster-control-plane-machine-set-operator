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

package framework

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DefaultTimeout is the default timeout for eventually and consistently assertions.
	DefaultTimeout = 60 * time.Second

	// DefaultInterval is the default interval for eventually and consistently assertions.
	DefaultInterval = 5 * time.Second
)

var (
	errContextCancelled = errors.New("context cancelled")
)

// GomegaAssertions is a subset of the gomega.Gomega interface.
// It is the set allowed for checks and conditions in the RunCheckUntil
// helper function.
type GomegaAssertions interface {
	Ω(actual interface{}, extra ...interface{}) gomega.Assertion //nolint:asciicheck
	Expect(actual interface{}, extra ...interface{}) gomega.Assertion
	ExpectWithOffset(offset int, actual interface{}, extra ...interface{}) gomega.Assertion
}

// ControlPlaneMachineSetKey is the object key for fetching a control plane
// machine set.
func ControlPlaneMachineSetKey() runtimeclient.ObjectKey {
	return runtimeclient.ObjectKey{
		Namespace: "openshift-machine-api",
		Name:      "cluster",
	}
}

// Periodic is a periodic ginkgo label.
func Periodic() ginkgo.Labels {
	return ginkgo.Label("Periodic")
}

// PreSubmit is a presubmit ginkgo label.
func PreSubmit() ginkgo.Labels {
	return ginkgo.Label("PreSubmit")
}

// Async runs the test function as an asynchronous goroutine.
// If the test function returns false, the cancel will be called.
// This allows to cancel the context if the test function fails.
func Async(wg *sync.WaitGroup, cancel context.CancelFunc, testFunc func() bool) {
	wg.Add(1)

	go func() {
		defer ginkgo.GinkgoRecover()
		defer wg.Done()

		if !testFunc() {
			cancel()
		}
	}()
}

// RunCheckUntil runs the check function until the condition succeeds or the context is cancelled.
// If the check fails before the condition succeeds, the test will fail.
// The check and condition functions must use the passed Gomega for any assertions so that we can handle failures
// within the functions appropriately.
func RunCheckUntil(ctx context.Context, check, condition func(context.Context, GomegaAssertions) bool) bool {
	return gomega.Eventually(func() error {
		checkErr := runAssertion(ctx, check)
		conditionErr := runAssertion(ctx, condition)

		switch {
		case conditionErr == nil:
			// The until finally succeeded.
			return nil
		case checkErr != nil:
			// The check failed but the until has not completed.
			// Abort the check.
			return gomega.StopTrying("Check failed before condition succeeded").Wrap(checkErr)
		default:
			return conditionErr
		}
	}).WithContext(ctx).Should(gomega.Succeed())
}

// runAssertion runs the assertion function and returns an error if the assertion failed.
func runAssertion(ctx context.Context, assertion func(context.Context, GomegaAssertions) bool) error {
	select {
	case <-ctx.Done():
		return errContextCancelled
	default:
	}

	var err error

	g := gomega.NewGomega(func(message string, callerSkip ...int) {
		err = errors.New(message) //nolint:goerr113
	})

	if !assertion(ctx, g) {
		return err
	}

	return nil
}