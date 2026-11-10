// Licensed under the Universal Permissive License (UPL) Version 1.0.

// Command dispatch is the entrypoint of the published Speedle image.
// The image is built FROM scratch, which has no shell, so this process selects
// the command. With no arguments it prints the commands in the image. The first
// argument must be speedle-pms, speedle-ads or spctl; dispatch then replaces
// itself with that binary.
package main

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

var commandNames = []string{"speedle-pms", "speedle-ads", "spctl"}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, syscall.Exec))
}

func run(args []string, stdout, stderr io.Writer, execFn func(string, []string, []string) error) int {
	if len(args) == 0 {
		writeCommands(stdout)
		return 0
	}
	name := args[0]
	for _, known := range commandNames {
		if name != known {
			continue
		}
		path := "/" + name
		argv := append([]string{name}, args[1:]...)
		if err := execFn(path, argv, os.Environ()); err != nil {
			fmt.Fprintf(stderr, "exec %s: %v\n", path, err)
			return 127
		}
		return 0
	}
	fmt.Fprintf(stderr, "unknown command: %s\n", name)
	writeCommands(stderr)
	return 2
}

func writeCommands(w io.Writer) {
	for _, name := range commandNames {
		fmt.Fprintln(w, name)
	}
}
