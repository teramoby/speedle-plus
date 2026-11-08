//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package command

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var list bool
var configTokenStdin bool

const maxTokenLength = 64 * 1024

var (
	configExample = `
		# List all global flags
		spctl config --list
		
		# Set global flag "timeout" and "skipverify"
		spctl config timeout 500ms skipverify true

		# Save an API token without putting it in command arguments
		printf '%s\n' "$SPCTL_TOKEN" | spctl config token --stdin`
)

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config [--list] [args]",
		Short:   "List or set global flags",
		Example: configExample,
		Run:     configCommandFunc,
	}

	cmd.Flags().BoolVar(&list, "list", false, "list all config")
	cmd.Flags().BoolVar(&configTokenStdin, "stdin", false, "read the API token from standard input")
	return cmd
}

func readTokenFromStdin(r io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxTokenLength+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxTokenLength {
		return "", fmt.Errorf("API token exceeds %d bytes", maxTokenLength)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("API token is empty")
	}
	return token, nil
}

func configCommandFunc(cmd *cobra.Command, args []string) {
	if configTokenStdin {
		if list || len(args) != 1 || args[0] != "token" {
			fmt.Println("Failed to set config, --stdin requires exactly: config token --stdin")
			os.Exit(1)
		}
		token, err := readTokenFromStdin(cmd.InOrStdin())
		if err != nil {
			fmt.Printf("Failed to set config, %v\n", err)
			os.Exit(1)
		}
		args = []string{"token", token}
	}
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
				if strings.ContainsAny(val, "\r\n") {
					fmt.Println("Failed to set config, API token must be a single line")
					os.Exit(1)
				}
				cloudFlags[name] = val
				delete(flags, name)
				cloudChanged = true
			} else {
				flags[name] = val
				if _, shadowed := cloudFlags[name]; shadowed {
					delete(cloudFlags, name)
					cloudChanged = true
				}
			}
		}
		// Keep non-token settings in ~/.spctlconfig and API tokens in ~/.speedle.
		// Remove a non-token key from ~/.speedle when setting it so the newly
		// written legacy value is not shadowed on the next invocation.
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
