/*
Copyright 2026 Red Hat, Inc.

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

package tls

import (
	"crypto/tls"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	configv1 "github.com/openshift/api/config/v1"
)

var _ = Describe("TLS curve CLI overrides", func() {
	DescribeTable("should apply explicit curves without fetching cluster configuration",
		func(minVersion string, ciphers []string, curveIDs []int32, expectedVersion uint16, expectedCurves []tls.CurveID) {
			// A nil REST config proves CLI resolution does not need an API connection.
			result, err := ResolveTLSConfig(ctx, nil, minVersion, ciphers, curveIDs)
			Expect(err).NotTo(HaveOccurred())

			tlsCfg := &tls.Config{}
			result.TLSConfig(tlsCfg)
			Expect(tlsCfg.MinVersion).To(Equal(expectedVersion))
			Expect(tlsCfg.CurvePreferences).To(Equal(expectedCurves))
			if expectedVersion == tls.VersionTLS13 || len(ciphers) == 0 {
				Expect(tlsCfg.CipherSuites).To(BeNil())
			} else {
				Expect(tlsCfg.CipherSuites).To(Equal([]uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}))
			}

			Expect(result.TLSAdherencePolicy).To(BeEmpty())
			Expect(result.TLSProfileSpec).To(Equal(configv1.TLSProfileSpec{}))
		},
		Entry("curve-only override uses TLS 1.2 and Go cipher defaults", "", []string(nil),
			[]int32{int32(tls.CurveP256), int32(tls.CurveP384)}, uint16(tls.VersionTLS12),
			[]tls.CurveID{tls.CurveP256, tls.CurveP384}),
		Entry("TLS 1.2 override with all three flags", "VersionTLS12",
			[]string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"}, []int32{int32(tls.CurveP384)},
			uint16(tls.VersionTLS12), []tls.CurveID{tls.CurveP384}),
		Entry("TLS 1.3 override still ignores cipher suites", "VersionTLS13",
			[]string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"}, []int32{int32(tls.X25519MLKEM768), int32(tls.X25519)},
			uint16(tls.VersionTLS13), []tls.CurveID{tls.X25519MLKEM768, tls.X25519}),
	)

	DescribeTable("should reject invalid curve preferences with a flag-specific error",
		func(curveIDs []int32, reason string) {
			_, err := ResolveTLSConfig(ctx, nil, "", nil, curveIDs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid --tls-curve-preferences value"))
			Expect(err.Error()).To(ContainSubstring(reason))
		},
		Entry("zero", []int32{0}, "out of range"),
		Entry("negative", []int32{-1}, "out of range"),
		Entry("too large", []int32{65536}, "out of range"),
		Entry("unsupported by Go", []int32{65535}, "not supported"),
		Entry("duplicate", []int32{int32(tls.CurveP256), int32(tls.CurveP256)}, "duplicate"),
	)

	It("should leave curves unchanged when only the existing flag pair is provided", func() {
		result, err := ResolveTLSConfig(ctx, nil, "VersionTLS12", []string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"}, nil)
		Expect(err).NotTo(HaveOccurred())

		tlsCfg := &tls.Config{CurvePreferences: []tls.CurveID{tls.CurveP521}}
		result.TLSConfig(tlsCfg)
		Expect(tlsCfg.CurvePreferences).To(Equal([]tls.CurveID{tls.CurveP521}))
	})

	DescribeTable("should use the same override decision for resolution and watcher registration",
		func(minVersion string, ciphers []string, curves []int32, overridden bool) {
			Expect(HasTLSOverrides(minVersion, ciphers, curves)).To(Equal(overridden))
		},
		Entry("no flags", "", []string(nil), []int32(nil), false),
		Entry("empty slices", "", []string{}, []int32{}, false),
		Entry("minimum version", "VersionTLS12", []string(nil), []int32(nil), true),
		Entry("cipher suites", "", []string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"}, []int32(nil), true),
		Entry("curve preferences alone", "", []string(nil), []int32{int32(tls.CurveP256)}, true),
		Entry("all flags", "VersionTLS12", []string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"}, []int32{int32(tls.CurveP256)}, true),
	)
})
