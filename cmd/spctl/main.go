//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package main

import (
	"github.com/teramoby/speedle-plus/cmd/spctl/command"
)

var gitCommit string
var productVersion string
var goVersion string

func main() {
	command.GitCommit = gitCommit
	command.ProductVersion = productVersion
	command.GoVersion = goVersion
	command.Execute()
}
