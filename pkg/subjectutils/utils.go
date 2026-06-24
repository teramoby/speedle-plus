//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package subjectutils

import (
	"fmt"
	"strings"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
)

// EncodePrincipal encodes prinicpal object to string
// Form: [idd=<IDD>:]<Type>:<Name>
// Returns error if principal names contain reserved separator characters (colon, equals).
func EncodePrincipal(principal *adsapi.Principal) (string, error) {
	if principal == nil {
		return "", fmt.Errorf("principal is nil")
	}
	// Reject colons in Type (which is the first field in the encoded format),
	// and equals signs in IDD (which is the key=value prefix).
	if strings.ContainsAny(principal.Type, ":") || strings.Contains(principal.IDD, "=") {
		return "", fmt.Errorf("principal type %q or IDD %q contains reserved characters (colon or equals)", principal.Type, principal.IDD)
	}
	if len(principal.IDD) != 0 {
		return fmt.Sprintf("idd=%s:%s:%s", principal.IDD, principal.Type, principal.Name), nil
	}
	return fmt.Sprintf("%s:%s", principal.Type, principal.Name), nil
}
