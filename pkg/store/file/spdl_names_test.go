//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package file

import "testing"

func TestPhaseAndLineTypeNames(t *testing.T) {
	// Unknown and out-of-range values used to index past the phase names.
	for value, want := range map[phase]string{phaseRoot: "phaseRoot", phaseRolepolicy: "phaseRolepolicy", phaseUnknown: "phaseUnknown", phase(9): "phaseUnknown"} {
		if got := value.String(); got != want {
			t.Errorf("phase(%d) = %q, want %q", int(value), got, want)
		}
	}
	for value, want := range map[lineType]string{lineEmpty: "lineEmpty", linePolicyDef: "linePolicyDef", lineUnknown: "lineUnknown", lineType(9): "lineUnknown"} {
		if got := value.String(); got != want {
			t.Errorf("lineType(%d) = %q, want %q", int(value), got, want)
		}
	}
}
