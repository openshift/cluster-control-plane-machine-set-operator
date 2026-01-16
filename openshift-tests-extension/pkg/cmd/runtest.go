package cmd

import (
	"bufio"
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

func NewRunTestCommand(registry *extension.Registry) *cobra.Command {
	opts := struct {
		componentFlags   *vendorflags.ComponentFlags
		concurrencyFlags *vendorflags.ConcurrencyFlags
		nameFlags        *vendorflags.NamesFlags
		outputFlags      *vendorflags.OutputFlags
		shardFlags       *flags.ShardFlags
	}{
		componentFlags:   vendorflags.NewComponentFlags(),
		nameFlags:        vendorflags.NewNamesFlags(),
		outputFlags:      vendorflags.NewOutputFlags(),
		concurrencyFlags: vendorflags.NewConcurrencyFlags(),
		shardFlags:       flags.NewShardFlags(),
	}

	cmd := &cobra.Command{
		Use:          "run-test [-n NAME...] [NAME]",
		Short:        "Runs tests by name with optional sharding support",
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
			if len(args) > 1 {
				return fmt.Errorf("use --names to specify more than one test")
			}
			opts.nameFlags.Names = append(opts.nameFlags.Names, args...)

			// allow reading tests from an stdin pipe
			info, err := os.Stdin.Stat()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeCharDevice == 0 { // Check if input is from a pipe
				scanner := bufio.NewScanner(os.Stdin)
				for scanner.Scan() {
					opts.nameFlags.Names = append(opts.nameFlags.Names, scanner.Text())
				}
				if err := scanner.Err(); err != nil {
					return fmt.Errorf("error reading from stdin: %v", err)
				}
			}

			if len(opts.nameFlags.Names) == 0 {
				return fmt.Errorf("must specify at least one test")
			}

			specs, err := ext.FindSpecsByName(opts.nameFlags.Names...)
			if err != nil {
				return err
			}

			// Apply sharding if enabled
			if opts.shardFlags.IsEnabled() {
				originalCount := len(specs)
				specs = shard.FilterSpecsByShard(specs, opts.shardFlags.ShardIndex, opts.shardFlags.ShardTotal)
				fmt.Fprintf(os.Stderr, "Sharding enabled: shard %d/%d, running %d of %d tests\n",
					opts.shardFlags.ShardIndex, opts.shardFlags.ShardTotal, len(specs), originalCount)
			}

			w, err := extensiontests.NewJSONResultWriter(os.Stdout, extensiontests.ResultFormat(opts.outputFlags.Output))
			if err != nil {
				return err
			}
			defer w.Flush()

			return specs.Run(ctx, w, opts.concurrencyFlags.MaxConcurency)
		},
	}
	opts.componentFlags.BindFlags(cmd.Flags())
	opts.nameFlags.BindFlags(cmd.Flags())
	opts.outputFlags.BindFlags(cmd.Flags())
	opts.concurrencyFlags.BindFlags(cmd.Flags())
	opts.shardFlags.BindFlags(cmd.Flags())

	return cmd
}
