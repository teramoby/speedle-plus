//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version variables set by the main package at startup.
var (
	GitCommit      string
	ProductVersion string
	GoVersion      string
)

func newVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "version",
		Short:   "Print version information",
		Example: "spctl version",
		Run:     versionCommand,
	}

	return cmd
}

func versionCommand(cmd *cobra.Command, args []string) {
	fmt.Printf("spctl:\n")
	fmt.Printf(" Version:       %s\n", ProductVersion)
	fmt.Printf(" Go Version:    %s\n", GoVersion)
	fmt.Printf(" Git commit:    %s\n", GitCommit)
}
