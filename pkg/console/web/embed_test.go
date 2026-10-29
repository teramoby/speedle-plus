//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package web

import (
	"io/fs"
	"testing"
)

func TestStaticHasThePage(t *testing.T) {
	for _, name := range []string{"index.html", "app.js", "style.css"} {
		if _, err := fs.Stat(Static(), name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
