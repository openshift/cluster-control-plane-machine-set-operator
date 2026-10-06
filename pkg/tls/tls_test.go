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

package tls

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	configv1 "github.com/openshift/api/config/v1"
	utiltls "github.com/openshift/controller-runtime-common/pkg/tls"
	libgocrypto "github.com/openshift/library-go/pkg/crypto"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ResolveTLSConfig", func() {
	Context("when CLI flags override the cluster-wide TLS profile", func() {
		It("should apply TLS 1.2 with the specified cipher suites", func() {
			result, err := ResolveTLSConfig(ctx, cfg, "VersionTLS12", []string{
				"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
				"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
			})
			Expect(err).NotTo(HaveOccurred())

			tlsCfg := &tls.Config{}
			result.TLSConfig(tlsCfg)

			Expect(tlsCfg.MinVersion).To(Equal(uint16(tls.VersionTLS12)))
			Expect(tlsCfg.CipherSuites).To(Equal([]uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			}))
		})

		It("should not set cipher suites when TLS 1.3 is specified", func() {
			result, err := ResolveTLSConfig(ctx, cfg, "VersionTLS13", []string{
				"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
			})
			Expect(err).NotTo(HaveOccurred())

			tlsCfg := &tls.Config{}
			result.TLSConfig(tlsCfg)

			Expect(tlsCfg.MinVersion).To(Equal(uint16(tls.VersionTLS13)))
			Expect(tlsCfg.CipherSuites).To(BeNil())
		})

		It("should not populate TLSAdherencePolicy or TLSProfileSpec", func() {
			result, err := ResolveTLSConfig(ctx, cfg, "VersionTLS12", []string{
				"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(result.TLSAdherencePolicy).To(BeEmpty())
			Expect(result.TLSProfileSpec).To(Equal(configv1.TLSProfileSpec{}))
		})

		It("should return an error for an invalid TLS version", func() {
			_, err := ResolveTLSConfig(ctx, cfg, "InvalidVersion", []string{
				"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid --tls-min-version"))
		})

		It("should return an error for an invalid cipher suite", func() {
			_, err := ResolveTLSConfig(ctx, cfg, "VersionTLS12", []string{
				"INVALID_CIPHER_SUITE",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid --tls-cipher-suites"))
		})
	})

	Context("when using the cluster-wide TLS profile", func() {
		AfterEach(func() {
			apiServer := &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
			}
			Eventually(func() bool {
				err := k8sClient.Delete(ctx, apiServer)
				return err == nil || apierrors.IsNotFound(err)
			}).Should(BeTrue())
		})

		It("should use the cluster profile when adherence is StrictAllComponents", func() {
			apiServer := &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileOldType,
					},
					TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
				},
			}
			Eventually(func() error {
				return k8sClient.Create(ctx, apiServer)
			}).Should(Succeed())

			result, err := ResolveTLSConfig(ctx, cfg, "", nil)
			Expect(err).NotTo(HaveOccurred())

			tlsCfg := &tls.Config{}
			result.TLSConfig(tlsCfg)

			oldProfile := configv1.TLSProfiles[configv1.TLSProfileOldType]
			expectedMinVersion := libgocrypto.TLSVersionOrDie(string(oldProfile.MinTLSVersion))
			Expect(tlsCfg.MinVersion).To(Equal(expectedMinVersion))
			Expect(tlsCfg.CipherSuites).NotTo(BeEmpty())
			Expect(tlsCfg.CurvePreferences).To(Equal(defaultCurvePreferences()))

			Expect(result.TLSAdherencePolicy).To(Equal(configv1.TLSAdherencePolicyStrictAllComponents))
			Expect(result.TLSProfileSpec).To(Equal(*oldProfile))
		})

		It("should use the default profile when adherence is NoOpinion", func() {
			apiServer := &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileOldType,
					},
				},
			}
			Eventually(func() error {
				return k8sClient.Create(ctx, apiServer)
			}).Should(Succeed())

			result, err := ResolveTLSConfig(ctx, cfg, "", nil)
			Expect(err).NotTo(HaveOccurred())

			tlsCfg := &tls.Config{}
			result.TLSConfig(tlsCfg)

			// NoOpinion means the default (Intermediate) profile is applied,
			// even though the Old profile is configured on the APIServer.
			defaultProfile := configv1.TLSProfiles[libgocrypto.DefaultTLSProfileType]
			expectedMinVersion := libgocrypto.TLSVersionOrDie(string(defaultProfile.MinTLSVersion))
			Expect(tlsCfg.MinVersion).To(Equal(expectedMinVersion))
			Expect(tlsCfg.CurvePreferences).To(Equal(defaultCurvePreferences()))

			// TLSProfileSpec still reflects what was fetched from the cluster.
			oldProfile := configv1.TLSProfiles[configv1.TLSProfileOldType]
			Expect(result.TLSProfileSpec).To(Equal(*oldProfile))
		})

		It("should use the default profile when adherence is LegacyAdheringComponentsOnly", func() {
			apiServer := &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileOldType,
					},
					TLSAdherence: configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly,
				},
			}
			Eventually(func() error {
				return k8sClient.Create(ctx, apiServer)
			}).Should(Succeed())

			result, err := ResolveTLSConfig(ctx, cfg, "", nil)
			Expect(err).NotTo(HaveOccurred())

			tlsCfg := &tls.Config{}
			result.TLSConfig(tlsCfg)

			defaultProfile := configv1.TLSProfiles[libgocrypto.DefaultTLSProfileType]
			expectedMinVersion := libgocrypto.TLSVersionOrDie(string(defaultProfile.MinTLSVersion))
			Expect(tlsCfg.MinVersion).To(Equal(expectedMinVersion))
			Expect(tlsCfg.CurvePreferences).To(Equal(defaultCurvePreferences()))

			Expect(result.TLSAdherencePolicy).To(Equal(configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly))
		})

		It("should apply a custom TLS profile when adherence is StrictAllComponents", func() {
			customCiphers := []string{"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-RSA-AES256-GCM-SHA384"}
			apiServer := &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileCustomType,
						Custom: &configv1.CustomTLSProfile{
							TLSProfileSpec: configv1.TLSProfileSpec{
								Ciphers:       customCiphers,
								MinTLSVersion: configv1.VersionTLS12,
							},
						},
					},
					TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
				},
			}
			Eventually(func() error {
				return k8sClient.Create(ctx, apiServer)
			}).Should(Succeed())

			result, err := ResolveTLSConfig(ctx, cfg, "", nil)
			Expect(err).NotTo(HaveOccurred())

			tlsCfg := &tls.Config{}
			result.TLSConfig(tlsCfg)

			Expect(tlsCfg.MinVersion).To(Equal(uint16(tls.VersionTLS12)))
			Expect(tlsCfg.CipherSuites).NotTo(BeEmpty())

			Expect(tlsCfg.CurvePreferences).To(BeNil())

			// An absent groups field must not overwrite the server's curve defaults.
			tlsCfg.CurvePreferences = []tls.CurveID{tls.CurveP384}
			result.TLSConfig(tlsCfg)
			Expect(tlsCfg.CurvePreferences).To(Equal([]tls.CurveID{tls.CurveP384}))

			Expect(result.TLSProfileSpec.Ciphers).To(Equal(customCiphers))
			Expect(result.TLSProfileSpec.MinTLSVersion).To(Equal(configv1.VersionTLS12))
		})

		It("should gracefully default when no APIServer resource exists", func() {
			result, err := ResolveTLSConfig(ctx, cfg, "", nil)
			Expect(err).NotTo(HaveOccurred())

			tlsCfg := &tls.Config{}
			result.TLSConfig(tlsCfg)

			// Falls back to the Intermediate profile when the APIServer is missing.
			defaultProfile := configv1.TLSProfiles[libgocrypto.DefaultTLSProfileType]
			expectedMinVersion := libgocrypto.TLSVersionOrDie(string(defaultProfile.MinTLSVersion))
			Expect(tlsCfg.MinVersion).To(Equal(expectedMinVersion))
			Expect(tlsCfg.CurvePreferences).To(Equal(defaultCurvePreferences()))

			Expect(result.TLSAdherencePolicy).To(BeEmpty())
			Expect(result.TLSProfileSpec).To(Equal(configv1.TLSProfileSpec{}))
		})

		DescribeTable("should apply configured groups when adherence is StrictAllComponents",
			func(version configv1.TLSProtocolVersion, groups []configv1.TLSGroup, curves []tls.CurveID) {
				apiServer := customAPIServer(version, groups, configv1.TLSAdherencePolicyStrictAllComponents)
				Expect(k8sClient.Create(ctx, apiServer)).To(Succeed())

				result, err := ResolveTLSConfig(ctx, cfg, "", nil)
				Expect(err).NotTo(HaveOccurred())

				tlsCfg := &tls.Config{}
				result.TLSConfig(tlsCfg)
				Expect(tlsCfg.CurvePreferences).To(Equal(curves))
				Expect(tlsCfg.MinVersion).To(Equal(libgocrypto.TLSVersionOrDie(string(version))))
				if version == configv1.VersionTLS13 {
					Expect(tlsCfg.CipherSuites).To(BeNil())
				} else {
					Expect(tlsCfg.CipherSuites).To(Equal([]uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}))
				}

				Expect(result.TLSProfileSpec.Groups).To(Equal(groups))
				persisted := &configv1.APIServer{}
				Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "cluster"}, persisted)).To(Succeed())
				Expect(persisted.Spec.TLSSecurityProfile.Custom.Groups).To(Equal(groups))
			},
			Entry("TLS 1.2 with NIST curves", configv1.VersionTLS12,
				[]configv1.TLSGroup{configv1.TLSGroupSecP256r1, configv1.TLSGroupSecP384r1, configv1.TLSGroupSecP521r1},
				[]tls.CurveID{tls.CurveP256, tls.CurveP384, tls.CurveP521}),
			Entry("TLS 1.3 with NIST curves", configv1.VersionTLS13,
				[]configv1.TLSGroup{configv1.TLSGroupSecP256r1, configv1.TLSGroupSecP384r1},
				[]tls.CurveID{tls.CurveP256, tls.CurveP384}),
			Entry("TLS 1.3 with hybrid key exchange", configv1.VersionTLS13,
				[]configv1.TLSGroup{configv1.TLSGroupX25519MLKEM768, configv1.TLSGroupX25519},
				[]tls.CurveID{tls.X25519MLKEM768, tls.X25519}),
		)

		DescribeTable("should ignore custom groups when adherence does not require the cluster profile",
			func(policy configv1.TLSAdherencePolicy) {
				apiServer := customAPIServer(configv1.VersionTLS12, []configv1.TLSGroup{configv1.TLSGroupSecP521r1}, policy)
				Expect(k8sClient.Create(ctx, apiServer)).To(Succeed())

				result, err := ResolveTLSConfig(ctx, cfg, "", nil)
				Expect(err).NotTo(HaveOccurred())

				tlsCfg := &tls.Config{}
				result.TLSConfig(tlsCfg)
				Expect(tlsCfg.CurvePreferences).To(Equal(defaultCurvePreferences()))
				Expect(result.TLSProfileSpec.Groups).To(Equal(apiServer.Spec.TLSSecurityProfile.Custom.Groups))
			},
			Entry("NoOpinion", configv1.TLSAdherencePolicyNoOpinion),
			Entry("LegacyAdheringComponentsOnly", configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly),
		)

		It("should continue to bypass cluster groups when TLS is overridden via CLI flags", func() {
			apiServer := customAPIServer(configv1.VersionTLS13,
				[]configv1.TLSGroup{configv1.TLSGroupSecP521r1}, configv1.TLSAdherencePolicyStrictAllComponents)
			Expect(k8sClient.Create(ctx, apiServer)).To(Succeed())

			result, err := ResolveTLSConfig(ctx, cfg, "VersionTLS12", []string{"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"})
			Expect(err).NotTo(HaveOccurred())

			tlsCfg := &tls.Config{}
			result.TLSConfig(tlsCfg)
			Expect(tlsCfg.MinVersion).To(Equal(uint16(tls.VersionTLS12)))
			Expect(tlsCfg.CipherSuites).To(Equal([]uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}))
			Expect(tlsCfg.CurvePreferences).To(BeNil())
			Expect(result.TLSAdherencePolicy).To(BeEmpty())
			Expect(result.TLSProfileSpec).To(Equal(configv1.TLSProfileSpec{}))
		})

		It("should trigger shutdown and reload the configuration after a groups-only change", func() {
			apiServer := customAPIServer(configv1.VersionTLS13,
				[]configv1.TLSGroup{configv1.TLSGroupSecP256r1}, configv1.TLSAdherencePolicyStrictAllComponents)
			Expect(k8sClient.Create(ctx, apiServer)).To(Succeed())

			initial, err := ResolveTLSConfig(ctx, cfg, "", nil)
			Expect(err).NotTo(HaveOccurred())

			shutdownCtx, cancel := context.WithCancel(ctx)
			defer cancel()

			var observedOld, observedNew configv1.TLSProfileSpec
			watcher := &utiltls.SecurityProfileWatcher{
				Client:                    k8sClient,
				InitialTLSProfileSpec:     initial.TLSProfileSpec,
				InitialTLSAdherencePolicy: initial.TLSAdherencePolicy,
				OnProfileChange: func(_ context.Context, oldProfile, newProfile configv1.TLSProfileSpec) {
					observedOld, observedNew = oldProfile, newProfile
					cancel()
				},
				OnAdherencePolicyChange: func(_ context.Context, _, _ configv1.TLSAdherencePolicy) {
					Fail("A groups-only update must not change the adherence policy")
				},
			}
			req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "cluster"}}
			_, err = watcher.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(shutdownCtx.Err()).NotTo(HaveOccurred())

			apiServer.Spec.TLSSecurityProfile.Custom.Groups = []configv1.TLSGroup{configv1.TLSGroupSecP384r1}
			Expect(k8sClient.Update(ctx, apiServer)).To(Succeed())
			_, err = watcher.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(shutdownCtx.Err()).To(MatchError(context.Canceled))
			Expect(observedOld).To(Equal(initial.TLSProfileSpec))
			Expect(observedNew.Groups).To(Equal([]configv1.TLSGroup{configv1.TLSGroupSecP384r1}))
			Expect(observedNew.Ciphers).To(Equal(observedOld.Ciphers))
			Expect(observedNew.MinTLSVersion).To(Equal(observedOld.MinTLSVersion))

			reloaded, err := ResolveTLSConfig(ctx, cfg, "", nil)
			Expect(err).NotTo(HaveOccurred())

			tlsCfg := &tls.Config{}
			reloaded.TLSConfig(tlsCfg)
			Expect(tlsCfg.CurvePreferences).To(Equal([]tls.CurveID{tls.CurveP384}))
			Expect(reloaded.TLSProfileSpec).To(Equal(observedNew))
		})

		It("should populate TLSAdherencePolicy and TLSProfileSpec from the cluster", func() {
			apiServer := &configv1.APIServer{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
				Spec: configv1.APIServerSpec{
					TLSSecurityProfile: &configv1.TLSSecurityProfile{
						Type: configv1.TLSProfileOldType,
					},
					TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
				},
			}
			Eventually(func() error {
				return k8sClient.Create(ctx, apiServer)
			}).Should(Succeed())

			result, err := ResolveTLSConfig(ctx, cfg, "", nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(result.TLSAdherencePolicy).To(Equal(configv1.TLSAdherencePolicyStrictAllComponents))
			Expect(result.TLSProfileSpec).To(Equal(*configv1.TLSProfiles[configv1.TLSProfileOldType]))
		})
	})
})

var _ = Describe("TLS group compatibility", func() {
	DescribeTable("should report unsupported groups without applying them",
		func(groups []configv1.TLSGroup, expectedCurves []tls.CurveID, expectedUnsupported []string) {
			// Unknown groups cannot be persisted through the current CRD's enum validation.
			// Exercise the shared helper directly to cover forward-compatible profile handling.
			applyTLS, unsupported := utiltls.NewTLSConfigFromProfile(configv1.TLSProfileSpec{
				MinTLSVersion: configv1.VersionTLS13,
				Groups:        groups,
			})
			Expect(unsupported).To(Equal(expectedUnsupported))

			tlsCfg := &tls.Config{}
			applyTLS(tlsCfg)
			Expect(tlsCfg.CurvePreferences).To(Equal(expectedCurves))
		},
		Entry("mixed supported and unsupported groups",
			[]configv1.TLSGroup{configv1.TLSGroupSecP256r1, "unknown-group", configv1.TLSGroupX25519MLKEM768},
			[]tls.CurveID{tls.CurveP256, tls.X25519MLKEM768}, []string{"unknown-group"}),
		Entry("only unsupported groups retain Go defaults",
			[]configv1.TLSGroup{"unknown-group"}, []tls.CurveID(nil), []string{"unknown-group"}),
	)

	DescribeTable("should enforce the resolved groups during a serving TLS handshake",
		func(version configv1.TLSProtocolVersion) {
			apiServer := customAPIServer(version,
				[]configv1.TLSGroup{configv1.TLSGroupSecP256r1}, configv1.TLSAdherencePolicyStrictAllComponents)
			Expect(k8sClient.Create(ctx, apiServer)).To(Succeed())
			DeferCleanup(func() {
				Expect(k8sClient.Delete(ctx, apiServer)).To(Succeed())
			})

			result, err := ResolveTLSConfig(ctx, cfg, "", nil)
			Expect(err).NotTo(HaveOccurred())

			// Metrics and webhooks both receive this same callback through TLSOpts.
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			server.TLS = &tls.Config{}
			result.TLSConfig(server.TLS)
			server.StartTLS()
			DeferCleanup(server.Close)

			dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			dialer := &tls.Dialer{Config: &tls.Config{
				// The test server uses a self-signed certificate; only group negotiation is under test.
				InsecureSkipVerify: true,
				MinVersion:         server.TLS.MinVersion,
				MaxVersion:         server.TLS.MinVersion,
				CurvePreferences:   []tls.CurveID{tls.CurveP256},
			}}
			conn, err := dialer.DialContext(dialCtx, "tcp", server.Listener.Addr().String())
			Expect(err).NotTo(HaveOccurred())
			Expect(conn.Close()).To(Succeed())

			dialer.Config = dialer.Config.Clone()
			dialer.Config.CurvePreferences = []tls.CurveID{tls.X25519}
			conn, err = dialer.DialContext(dialCtx, "tcp", server.Listener.Addr().String())
			if conn != nil {
				Expect(conn.Close()).To(Succeed())
			}
			Expect(err).To(HaveOccurred())
		},
		Entry("TLS 1.2", configv1.VersionTLS12),
		Entry("TLS 1.3", configv1.VersionTLS13),
	)
})

// customAPIServer builds a custom profile used to exercise group configuration through the API.
func customAPIServer(version configv1.TLSProtocolVersion, groups []configv1.TLSGroup, policy configv1.TLSAdherencePolicy) *configv1.APIServer {
	return &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSAdherence: policy,
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: version,
						Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
						Groups:        groups,
					},
				},
			},
		},
	}
}

// defaultCurvePreferences lists the groups in the current built-in TLS profiles.
func defaultCurvePreferences() []tls.CurveID {
	return []tls.CurveID{tls.X25519MLKEM768, tls.X25519, tls.CurveP256, tls.CurveP384}
}
