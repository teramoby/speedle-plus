//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package command

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var list bool

var (
	configExample = `
		# List all global flags
		spctl config --list
		
		# Set global flag "timeout" and "skipverify"
		spctl config timeout 500ms skipverify true`
)

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config [--list] [args]",
		Short:   "List or set global flags",
		Example: configExample,
		Run:     configCommandFunc,
	}

	cmd.Flags().BoolVar(&list, "list", false, "list all config")
	return cmd
}

func configCommandFunc(cmd *cobra.Command, args []string) {
	if list {
		// setflagsFromConfigFile(cmd)
		fs := cmd.InheritedFlags()
		fs.VisitAll(func(f *pflag.Flag) {
			if f.Hidden {
				return
			}
			value := f.Value.String()
			if f.Name == "token" && value != "" {
				value = "********"
			}
			fmt.Printf("%v = %v\n", f.Name, value)
		})
	} else {
		flags, err := readConfig(configFile)
		if err != nil {
			fmt.Printf("Failed to set config, %v\n", err)
			os.Exit(1)
		}
		cloudFlags, err := readConfig(cloudConfigFile)
		if err != nil {
			fmt.Printf("Failed to set config, %v\n", err)
			os.Exit(1)
		}
		cloudChanged := false
		for i := 0; i+1 < len(args); i += 2 {
			name := args[i]
			val := args[i+1]
			if name == "token" {
				cloudFlags[name] = val
				cloudChanged = true
			} else {
				flags[name] = val
			}
		}
		// Keep writing existing settings to ~/.spctlconfig. API tokens and any
		// other Cloud settings in ~/.speedle stay in that separate file.
		if err = writeConfigFile(flags); err != nil {
			fmt.Printf("Failed to set config, %v\n", err)
			os.Exit(1)
		}
		if cloudChanged {
			if err = writeCloudConfigFile(cloudFlags); err != nil {
				fmt.Printf("Failed to set config, %v\n", err)
				os.Exit(1)
			}
		}
	}
}
