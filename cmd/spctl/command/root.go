//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package command

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/teramoby/speedle-plus/pkg/cmd/flags"
)

const (
	cliName        = "spctl"
	cliDescription = "A command line interface for speedle"
	defaultTimeout = 5 * time.Second
)

var rootCmd = &cobra.Command{
	Use:        cliName,
	Short:      cliDescription,
	SuggestFor: []string{"spctl"},
}

func printHelpAndExit(cmd *cobra.Command) {
	cmd.Help()
	os.Exit(1)
}

func init() {
	rootCmd.PersistentFlags().StringVar(&globalFlags.PMSEndpoint, "pms-endpoint", flags.DefaultPolicyMgmtEndPoint, "speedle policy managemnet service endpoint")
	rootCmd.PersistentFlags().DurationVar(&globalFlags.Timeout, "timeout", 5000000000, "timeout for running command")
	rootCmd.PersistentFlags().StringVar(&globalFlags.CertFile, "cert", "", "identify secure client using this TLS certificate file")
	rootCmd.PersistentFlags().StringVar(&globalFlags.KeyFile, "key", "", "identify secure client using this TLS key file")
	rootCmd.PersistentFlags().StringVar(&globalFlags.CAFile, "cacert", "", "verify certificates of TLS-enabled secure servers using this CA bundle")
	rootCmd.PersistentFlags().BoolVar(&globalFlags.InsecureSkipVerify, "skipverify", false, "control whether a client verifies the server's certificate chain and host name or not")

	args, _ := readConfigFile()
	for name, val := range args {
		rootCmd.PersistentFlags().Set(name, val)
	}

	rootCmd.AddCommand(
		newGetCommand(),
		newDeleteCommand(),
		newCreateCommand(),
		newConfigCommand(),
		newDiscoverCommand(),
		newVersionCommand(),
	)
}

// Execute is the main function to execute commands
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
