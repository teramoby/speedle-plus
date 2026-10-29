//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

// Package web holds the console's page, built into the binary.
package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var files embed.FS

// static cannot fail: the directory is embedded above.
var static, _ = fs.Sub(files, "static")

// Static returns the page's files, with index.html at the root.
func Static() fs.FS { return static }
