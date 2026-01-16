package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openshift/cluster-control-plane-machine-set-operator/openshift-tests-extension/pkg/flags"
	"github.com/openshift/cluster-control-plane-machine-set-operator/openshift-tests-extension/pkg/shard"

	"github.com/openshift-eng/openshift-tests-extension/pkg/extension"
	"github.com/openshift-eng/openshift-tests-extension/pkg/extension/extensiontests"
	vendorflags "github.com/openshift-eng/openshift-tests-extension/pkg/flags"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func NewRunSuiteCommand(registry *extension.Registry) *cobra.Command {
	opts := struct {
		componentFlags   *vendorflags.ComponentFlags
		outputFlags      *vendorflags.OutputFlags
		concurrencyFlags *vendorflags.ConcurrencyFlags
		shardFlags       *flags.ShardFlags
		junitPath        string
	}{
		componentFlags:   vendorflags.NewComponentFlags(),
		outputFlags:      vendorflags.NewOutputFlags(),
		concurrencyFlags: vendorflags.NewConcurrencyFlags(),
		shardFlags:       flags.NewShardFlags(),
		junitPath:        "",
	}

	cmd := &cobra.Command{
		Use:          "run-suite NAME",
		Short:        "Run a group of tests by suite with optional sharding support",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate shard configuration
			if err := opts.shardFlags.Validate(); err != nil {
				return err
			}

			ctx, cancelCause := context.WithCancelCause(context.Background())
			defer cancelCause(errors.New("exiting"))

			abortCh := make(chan os.Signal, 2)
			go func() {
				<-abortCh
				fmt.Fprintf(os.Stderr, "Interrupted, terminating tests\n")
				cancelCause(errors.New("interrupt received"))

				select {
				case sig := <-abortCh:
					fmt.Fprintf(os.Stderr, "Interrupted twice, exiting (%s)\n", sig)
					switch sig {
					case syscall.SIGINT:
						os.Exit(130)
					default:
						os.Exit(130)
					}
				case <-time.After(30 * time.Minute):
					fmt.Fprintf(os.Stderr, "Timed out during cleanup, exiting\n")
					os.Exit(130)
				}
			}()
			signal.Notify(abortCh, syscall.SIGINT, syscall.SIGTERM)

			ext := registry.Get(opts.componentFlags.Component)
			if ext == nil {
				return fmt.Errorf("component not found: %s", opts.componentFlags.Component)
			}
			if len(args) != 1 {
				return fmt.Errorf("must specify one suite name")
			}
			suite, err := ext.GetSuite(args[0])
			if err != nil {
				return errors.Wrapf(err, "couldn't find suite: %s", args[0])
			}

			compositeWriter := extensiontests.NewCompositeResultWriter()
			defer func() {
				if err = compositeWriter.Flush(); err != nil {
					fmt.Fprintf(os.Stderr, "failed to write results: %v\n", err)
				}
			}()

			// JUnit writer if needed
			if opts.junitPath != "" {
				junitWriter, err := extensiontests.NewJUnitResultWriter(opts.junitPath, suite.Name)
				if err != nil {
					return errors.Wrap(err, "couldn't create junit writer")
				}
				compositeWriter.AddWriter(junitWriter)
			}

			// JSON writer
			jsonWriter, err := extensiontests.NewJSONResultWriter(os.Stdout,
				extensiontests.ResultFormat(opts.outputFlags.Output))
			if err != nil {
				return err
			}
			compositeWriter.AddWriter(jsonWriter)

			specs, err := ext.GetSpecs().Filter(suite.Qualifiers)
			if err != nil {
				return errors.Wrap(err, "couldn't filter specs")
			}

			// Apply sharding if enabled
			if opts.shardFlags.IsEnabled() {
				originalCount := len(specs)
				specs = shard.FilterSpecsByShard(specs, opts.shardFlags.ShardIndex, opts.shardFlags.ShardTotal)
				fmt.Fprintf(os.Stderr, "Sharding enabled: shard %d/%d, running %d of %d tests\n",
					opts.shardFlags.ShardIndex, opts.shardFlags.ShardTotal, len(specs), originalCount)
			}

			return specs.Run(ctx, compositeWriter, opts.concurrencyFlags.MaxConcurency)
		},
	}

	opts.componentFlags.BindFlags(cmd.Flags())
	opts.outputFlags.BindFlags(cmd.Flags())
	opts.concurrencyFlags.BindFlags(cmd.Flags())
	opts.shardFlags.BindFlags(cmd.Flags())
	cmd.Flags().StringVarP(&opts.junitPath, "junit-path", "j", opts.junitPath, "write results to junit XML")

	return cmd
}
