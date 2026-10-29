//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package command

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/pdl"
	"github.com/teramoby/speedle-plus/pkg/store/file"
)

var checkFiles []string

var checkExample = `
		# Check one policy
		spctl check policy "grant user alice approve /orders if amount <= 1000"

		# Check one role policy
		spctl check rolepolicy "grant user alice manager on /orders"

		# Check policy store files, SPDL (.spdl) or JSON; every error is listed
		spctl check -f policies.spdl -f store.json`

func newCheckCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check (policy | rolepolicy) POLICY | check --file FILE...",
		Short: "Check policies or policy store files without connecting to PMS",
		Long: `Check that policies are valid SPDL and that their conditions compile, without
connecting to PMS. Every error is printed with where it is; the exit code is 1
when there is any error.`,
		Example: checkExample,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 && len(checkFiles) == 0 {
				printHelpAndExit(cmd)
			}
			os.Exit(runCheck(args, checkFiles, os.Stdout))
		},
	}
	cmd.Flags().StringArrayVarP(&checkFiles, "file", "f", nil, "policy store file to check, SPDL if it ends in .spdl and JSON otherwise; can be repeated")
	return cmd
}

// runCheck checks the policy in args and the files, writes the result to out,
// and returns the exit code.
func runCheck(args, files []string, out io.Writer) int {
	failed := false
	if len(args) > 0 {
		if !checkPolicyLine(args, out) {
			failed = true
		}
	}
	for _, name := range files {
		if !checkFile(name, out) {
			failed = true
		}
	}
	if failed {
		return 1
	}
	return 0
}

func checkPolicyLine(args []string, out io.Writer) bool {
	if len(args) < 2 {
		fmt.Fprintf(out, "%s: give the policy to check after it\n", args[0])
		return false
	}
	line := strings.Join(args[1:], " ")
	var err error
	switch strings.ToLower(args[0]) {
	case "policy":
		_, _, err = pdl.ParsePolicy(line, "")
	case "rolepolicy":
		_, _, err = pdl.ParseRolePolicy(line, "")
	default:
		fmt.Fprintf(out, "unknown kind %q: use policy or rolepolicy\n", args[0])
		return false
	}
	if err != nil {
		fmt.Fprintln(out, err)
		return false
	}
	fmt.Fprintf(out, "%s OK\n", args[0])
	return true
}

func checkFile(name string, out io.Writer) bool {
	f, err := os.Open(name)
	if err != nil {
		fmt.Fprintln(out, err)
		return false
	}
	defer f.Close()

	var ps *pms.PolicyStore
	var errs []error
	if strings.HasSuffix(name, ".spdl") {
		ps, errs = file.CheckSPDL(f, name)
	} else {
		ps, errs = file.CheckJSON(f, name)
	}
	for _, err := range errs {
		fmt.Fprintln(out, err)
	}
	if len(errs) > 0 {
		fmt.Fprintf(out, "%s: %d error(s)\n", name, len(errs))
		return false
	}
	policies, rolePolicies := 0, 0
	for _, service := range ps.Services {
		policies += len(service.Policies)
		rolePolicies += len(service.RolePolicies)
	}
	fmt.Fprintf(out, "%s: OK, %d service(s), %d policies, %d role policies\n", name, len(ps.Services), policies, rolePolicies)
	return true
}
