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
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/reporters"
	ginkgotypes "github.com/onsi/ginkgo/v2/types"
	"github.com/onsi/gomega"
)

const informingLabelName = "Informing"

// InformingReporter is a Ginkgo AfterSuiteReporter that marks tests with the Informing label as flakes.
func InformingReporter(report ginkgo.Report) {
	informingReport := ginkgo.Report{
		SuitePath:                 report.SuitePath,
		SuiteDescription:          report.SuiteDescription,
		SuiteLabels:               report.SuiteLabels,
		SuiteSucceeded:            true,
		SuiteHasProgrammaticFocus: report.SuiteHasProgrammaticFocus,
		SuiteConfig:               report.SuiteConfig,
	}

	for _, specReport := range report.SpecReports {
		if specReport.Failed() && func(labels []string, informingLabel string) bool {
			for _, l := range labels {
				if l == informingLabel {
					return true
				}
			}

			return false
		}(specReport.Labels(), informingLabelName) {
			specReport.State = ginkgotypes.SpecStatePassed
			specReport.Failure = ginkgotypes.Failure{}
			informingReport.SpecReports = append(informingReport.SpecReports, specReport)
		}
	}

	informingReportFile := informingJUnitReportFileName()

	gomega.Expect(reporters.GenerateJUnitReport(informingReport, informingReportFile)).To(gomega.Succeed())
}

// Informing is a ginkgo label to mark a particular test as informing.
// Informing tests are added to blocking suites, but are not considered blocking themselves.
// Once the test has proven to be stable, the informing label should be removed.
func Informing() ginkgo.Labels {
	return ginkgo.Label(informingLabelName)
}

func informingJUnitReportFileName() string {
	_, reporterConfig := ginkgo.GinkgoConfiguration()
	junitReportFile := reporterConfig.JUnitReport

	return strings.TrimSuffix(junitReportFile, ".xml") + ".informing.xml"
}
