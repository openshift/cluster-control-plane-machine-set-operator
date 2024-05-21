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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Async utils", func() {
	Context("RunCheckUntil", func() {
		It("Should return true when the condition passes", MustPassRepeatedly(5), func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			deadline := time.Now().Add(1 * time.Second)

			errs := []error{}
			fail := func(message string, callerSkip ...int) {
				errs = append(errs, fmt.Errorf(message)) //nolint:err113
			}

			// Register a temporary fail handler so we can check what failures occur
			// during the execution of RunCheckUntil.
			RegisterFailHandler(fail)

			runCheckResult := RunCheckUntil(
				ctx,
				func(ctx context.Context, g GomegaAssertions) bool {
					return g.Expect(true).Should(BeTrue())
				},
				func(ctx context.Context, g GomegaAssertions) bool {
					return g.Expect(time.Now().After(deadline)).Should(BeTrue())
				},
			)

			// Restore the original fail handler.
			RegisterFailHandler(Fail)

			Expect(errs).To(BeEmpty())
			Expect(runCheckResult).To(BeTrue())
		})

		It("Should return true when the check fails exactly when the condition passes", MustPassRepeatedly(5), func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			signal := make(chan struct{})

			errs := []error{}
			fail := func(message string, callerSkip ...int) {
				errs = append(errs, fmt.Errorf(message)) //nolint:err113
			}

			// Register a temporary fail handler so we can check what failures occur
			// during the execution of RunCheckUntil.
			RegisterFailHandler(fail)

			go func() {
				time.Sleep(1 * time.Second)
				close(signal)
			}()

			runCheckResult := RunCheckUntil(
				ctx,
				func(ctx context.Context, g GomegaAssertions) bool {
					return g.Expect(signal).ShouldNot(BeClosed())
				},
				func(ctx context.Context, g GomegaAssertions) bool {
					return g.Expect(signal).Should(BeClosed())
				},
			)

			// Restore the original fail handler.
			RegisterFailHandler(Fail)

			Expect(errs).To(BeEmpty())
			Expect(runCheckResult).To(BeTrue())
		})

		It("Should return false when the check fails before the condition passes", MustPassRepeatedly(5), func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			signal := make(chan struct{})

			errs := []error{}
			fail := func(message string, callerSkip ...int) {
				errs = append(errs, fmt.Errorf(message)) //nolint:err113
			}

			// Register a temporary fail handler so we can check what failures occur
			// during the execution of RunCheckUntil.
			RegisterFailHandler(fail)

			go func() {
				time.Sleep(1 * time.Second)
				close(signal)
			}()

			runCheckResult := RunCheckUntil(
				ctx,
				func(ctx context.Context, g GomegaAssertions) bool {
					return g.Expect(signal).ShouldNot(BeClosed())
				},
				func(ctx context.Context, g GomegaAssertions) bool {
					return g.Expect(true).Should(BeFalse())
				},
			)

			// Restore the original fail handler.
			RegisterFailHandler(Fail)

			Expect(errs).To(ConsistOf(MatchError(SatisfyAll(
				ContainSubstring("Told to stop trying"),
				ContainSubstring("Check failed before condition succeeded"),
			))))
			Expect(runCheckResult).To(BeFalse())
		})

		It("Should return false when the condition never passes", MustPassRepeatedly(5), func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			errs := []error{}
			fail := func(message string, callerSkip ...int) {
				errs = append(errs, fmt.Errorf(message)) //nolint:err113
			}

			// Register a temporary fail handler so we can check what failures occur
			// during the execution of RunCheckUntil.
			RegisterFailHandler(fail)

			runCheckResult := RunCheckUntil(
				ctx,
				func(ctx context.Context, g GomegaAssertions) bool {
					return g.Expect(true).Should(BeTrue())
				},
				func(ctx context.Context, g GomegaAssertions) bool {
					return g.Expect(true).Should(BeFalse())
				},
			)

			// Restore the original fail handler.
			RegisterFailHandler(Fail)

			Expect(runCheckResult).To(BeFalse())

			// The consistently error never gets reported to the fail handler because
			// we intercept it, only the eventually error is reported.
			Expect(errs).To(ConsistOf(MatchError(SatisfyAll(
				ContainSubstring("Context was cancelled"),
				ContainSubstring("check failed or condition did not succeed before the context was cancelled"),
				ContainSubstring("Expected success, but got an error"),
			))))
		})
	})
})
