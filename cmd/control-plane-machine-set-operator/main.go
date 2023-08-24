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

package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/pflag"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/component-base/config"
	"k8s.io/component-base/config/options"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	cpmscontroller "github.com/openshift/cluster-control-plane-machine-set-operator/pkg/controllers/controlplanemachineset"
	cpmsgeneratorcontroller "github.com/openshift/cluster-control-plane-machine-set-operator/pkg/controllers/controlplanemachinesetgenerator"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/util"
	cpmswebhook "github.com/openshift/cluster-control-plane-machine-set-operator/pkg/webhooks/controlplanemachineset"

	//+kubebuilder:scaffold:imports

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
)

const (
	// defaultLeaderElectionID is the default name to use for the leader election resource.
	defaultLeaderElectionID = "control-plane-machine-set-leader"
)

const (
	releaseVersionEnvVariableName = "RELEASE_VERSION"
	unknownVersionValue           = "unknown"
)

func main() { //nolint:funlen,cyclop
	scheme := runtime.NewScheme()
	setupLog := ctrl.Log.WithName("setup")

	if err := setupScheme(scheme); err != nil {
		setupLog.Error(err, "unable to set up scheme")
		os.Exit(1)
	}

	var (
		metricsAddr      string
		probeAddr        string
		webhookPort      int
		managedNamespace string

		leaderElectionConfig = config.LeaderElectionConfiguration{
			LeaderElect:  true,
			ResourceName: defaultLeaderElectionID,
		}

		// defaultSyncPeriod is the default period after which to trigger controller's cache resync.
		defaultSyncPeriod = 30 * time.Minute
	)

	pflag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	pflag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	pflag.IntVar(&webhookPort, "webhook-port", 9443, "Webhook Server port, enabled by default at port 9443. Set to 0 to disable webhooks.")
	pflag.StringVar(&managedNamespace, "namespace", "openshift-machine-api", "The namespace for managed objects, where the machines and control plane machine set will operate.")
	options.BindLeaderElectionFlags(&leaderElectionConfig, pflag.CommandLine)

	klog.InitFlags(flag.CommandLine)

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	ctrl.SetLogger(klogr.New())

	cfg := ctrl.GetConfigOrDie()
	le := util.GetLeaderElectionDefaults(cfg, configv1.LeaderElection{
		Disable:       !leaderElectionConfig.LeaderElect,
		RenewDeadline: leaderElectionConfig.RenewDeadline,
		RetryPeriod:   leaderElectionConfig.RetryPeriod,
		LeaseDuration: leaderElectionConfig.LeaseDuration,
	})

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                        scheme,
		MetricsBindAddress:            metricsAddr,
		Port:                          webhookPort,
		HealthProbeBindAddress:        probeAddr,
		LeaderElectionNamespace:       leaderElectionConfig.ResourceNamespace,
		LeaderElection:                leaderElectionConfig.LeaderElect,
		LeaderElectionID:              leaderElectionConfig.ResourceName,
		LeaderElectionReleaseOnCancel: true,
		LeaseDuration:                 &le.LeaseDuration.Duration,
		RetryPeriod:                   &le.RetryPeriod.Duration,
		RenewDeadline:                 &le.RenewDeadline.Duration,
		Namespace:                     managedNamespace,
		// Do a full resync to catch up in case of missing events.
		SyncPeriod: &defaultSyncPeriod,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Define an uncached client.
	// More resource intensive than the default client,
	// is to be used only in situations where we want to avoid the cache.
	// We specifically declare an uncached client.Client rather than a client.Reader
	// for it to be wire compatible with the default client, so that we can easily
	// override it as needed.
	uncachedClient, err := client.New(cfg, client.Options{Scheme: mgr.GetScheme(), Mapper: mgr.GetRESTMapper()})
	if err != nil {
		setupLog.Error(err, "unable to set up uncached client")
	}

	if err := (&cpmscontroller.ControlPlaneMachineSetReconciler{
		Client:         mgr.GetClient(),
		UncachedClient: client.NewNamespacedClient(uncachedClient, managedNamespace),
		Scheme:         mgr.GetScheme(),
		Namespace:      managedNamespace,
		OperatorName:   "control-plane-machine-set",
		ReleaseVersion: getReleaseVersion(setupLog),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ControlPlaneMachineSet")
		os.Exit(1)
	}

	if err := (&cpmsgeneratorcontroller.ControlPlaneMachineSetGeneratorReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Namespace: managedNamespace,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ControlPlaneMachineSetGenerator")
		os.Exit(1)
	}

	if webhookPort != 0 {
		if err := (&cpmswebhook.ControlPlaneMachineSetWebhook{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "ControlPlaneMachineSet")
			os.Exit(1)
		}
	}

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}

	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// setupScheme adds the various schemes required for this operator to the
// scheme for the manager.
func setupScheme(scheme *runtime.Scheme) error {
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return fmt.Errorf("unable to add client-go scheme: %w", err)
	}

	if err := machinev1.Install(scheme); err != nil {
		return fmt.Errorf("unable to add machine.openshift.io/v1 scheme: %w", err)
	}

	if err := machinev1beta1.Install(scheme); err != nil {
		return fmt.Errorf("unable to add machine.openshift.io/v1beta1 scheme: %w", err)
	}

	if err := configv1.Install(scheme); err != nil {
		return fmt.Errorf("unable to add config.openshift.io/v1 scheme: %w", err)
	}

	return nil
}

func getReleaseVersion(setupLog logr.Logger) string {
	releaseVersion := os.Getenv(releaseVersionEnvVariableName)
	if len(releaseVersion) == 0 {
		releaseVersion = unknownVersionValue
		setupLog.Info(fmt.Sprintf("%s environment variable is missing, defaulting to %q", releaseVersionEnvVariableName, unknownVersionValue))
	}

	return releaseVersion
}
