// Copyright (C) 2019-2024, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/ava-labs/avalanchego/tests"
	"github.com/ava-labs/avalanchego/tests/fixture/tmpnet"
	"github.com/ava-labs/avalanchego/utils/logging"
	"github.com/ava-labs/avalanchego/version"
)

const cliVersion = "0.0.1"

var (
	errAvalancheGoRequired = fmt.Errorf("--avalanchego-path or %s are required", tmpnet.AvalancheGoPathEnvName)
	errNetworkDirRequired  = fmt.Errorf("--network-dir or %s are required", tmpnet.NetworkDirEnvName)
)

func main() {
	var (
		networkDir   string
		rawLogFormat string
	)
	rootCmd := &cobra.Command{
		Use:   "tmpnetctl",
		Short: "tmpnetctl commands",
	}
	rootCmd.PersistentFlags().StringVar(&networkDir, "network-dir", os.Getenv(tmpnet.NetworkDirEnvName), "The path to the configuration directory of a temporary network")
	rootCmd.PersistentFlags().StringVar(&rawLogFormat, "log-format", logging.AutoString, logging.FormatDescription)

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version details",
		RunE: func(*cobra.Command, []string) error {
			msg := cliVersion
			if len(version.GitCommit) > 0 {
				msg += ", commit=" + version.GitCommit
			}
			fmt.Fprintln(os.Stdout, msg)
			return nil
		},
	}
	rootCmd.AddCommand(versionCmd)

	var (
		rootDir         string
		networkOwner    string
		avalancheGoPath string
		pluginDir       string
		nodeCount       uint8
	)
	startNetworkCmd := &cobra.Command{
		Use:   "start-network",
		Short: "Start a new temporary network",
		RunE: func(*cobra.Command, []string) error {
			if len(avalancheGoPath) == 0 {
				return errAvalancheGoRequired
			}

			log, err := tests.LoggerForFormat("", rawLogFormat)
			if err != nil {
				return err
			}

			// Root dir will be defaulted on start if not provided

			network := &tmpnet.Network{
				Owner: networkOwner,
				Nodes: tmpnet.NewNodesOrPanic(int(nodeCount)),
			}

			// Extreme upper bound, should never take this long
			networkStartTimeout := 2 * time.Minute

			ctx, cancel := context.WithTimeout(context.Background(), networkStartTimeout)
			defer cancel()
			if err := tmpnet.BootstrapNewNetwork(
				ctx,
				log,
				network,
				rootDir,
				avalancheGoPath,
				pluginDir,
			); err != nil {
				log.Error("failed to bootstrap network", zap.Error(err))
				return err
			}

			// Symlink the new network to the 'latest' network to simplify usage
			networkRootDir := filepath.Dir(network.Dir)
			networkDirName := filepath.Base(network.Dir)
			latestSymlinkPath := filepath.Join(networkRootDir, "latest")
			if err := os.Remove(latestSymlinkPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			if err := os.Symlink(networkDirName, latestSymlinkPath); err != nil {
				return err
			}

			fmt.Fprintln(os.Stdout, "\nConfigure tmpnetctl to target this network by default with one of the following statements:")
			fmt.Fprintf(os.Stdout, " - source %s\n", network.EnvFilePath())
			fmt.Fprintf(os.Stdout, " - %s\n", network.EnvFileContents())
			fmt.Fprintf(os.Stdout, " - export %s=%s\n", tmpnet.NetworkDirEnvName, latestSymlinkPath)

			return nil
		},
	}
	startNetworkCmd.PersistentFlags().StringVar(&rootDir, "root-dir", os.Getenv(tmpnet.RootDirEnvName), "The path to the root directory for temporary networks")
	startNetworkCmd.PersistentFlags().StringVar(&avalancheGoPath, "avalanchego-path", os.Getenv(tmpnet.AvalancheGoPathEnvName), "The path to an avalanchego binary")
	startNetworkCmd.PersistentFlags().StringVar(&pluginDir, "plugin-dir", os.ExpandEnv("$HOME/.avalanchego/plugins"), "[optional] the dir containing VM plugins")
	startNetworkCmd.PersistentFlags().Uint8Var(&nodeCount, "node-count", tmpnet.DefaultNodeCount, "Number of nodes the network should initially consist of")
	startNetworkCmd.PersistentFlags().StringVar(&networkOwner, "network-owner", "", "The string identifying the intended owner of the network")
	rootCmd.AddCommand(startNetworkCmd)

	stopNetworkCmd := &cobra.Command{
		Use:   "stop-network",
		Short: "Stop a temporary network",
		RunE: func(*cobra.Command, []string) error {
			if len(networkDir) == 0 {
				return errNetworkDirRequired
			}
			ctx, cancel := context.WithTimeout(context.Background(), tmpnet.DefaultNetworkTimeout)
			defer cancel()
			if err := tmpnet.StopNetwork(ctx, networkDir); err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, "Stopped network configured at: %s\n", networkDir)
			return nil
		},
	}
	rootCmd.AddCommand(stopNetworkCmd)

	restartNetworkCmd := &cobra.Command{
		Use:   "restart-network",
		Short: "Restart a temporary network",
		RunE: func(*cobra.Command, []string) error {
			if len(networkDir) == 0 {
				return errNetworkDirRequired
			}
			log, err := tests.LoggerForFormat("", rawLogFormat)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), tmpnet.DefaultNetworkTimeout)
			defer cancel()
			return tmpnet.RestartNetwork(ctx, log, networkDir)
		},
	}
	rootCmd.AddCommand(restartNetworkCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "tmpnetctl failed: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
