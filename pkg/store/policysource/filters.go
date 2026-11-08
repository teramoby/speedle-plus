// Licensed under the Universal Permissive License (UPL) Version 1.0.

package policysource

import (
	"strings"

	log "github.com/sirupsen/logrus"
)

// The name filter follows the shared store contract: "name <operator> <value>"
// with the operators eq, co, sw, pr, gt, ge, lt and le. An invalid filter
// matches everything, as it does in the other stores.

type filter struct {
	field    string
	operator string
	target   string
}

func parseFilter(filterStr string) *filter {
	if len(filterStr) == 0 {
		return nil
	}
	if !strings.HasPrefix(filterStr, "name") {
		log.Error("unsupported filter string:", filterStr)
		return nil
	}
	values := strings.Split(filterStr, " ")
	if len(values) < 2 {
		log.Error("invalid filter string:", filterStr)
		return nil
	}
	parsed := &filter{field: values[0], operator: values[1]}
	if len(values) > 2 {
		parsed.target = values[2]
	}
	return parsed
}

// nameFilter reports whether a name matches the filter. An invalid field or
// operator matches everything.
func nameFilter(name string, f *filter) bool {
	if f.field != "name" {
		log.Error("invalid name filter. filter is:", f)
		return true
	}
	switch f.operator {
	case "eq":
		return name == f.target
	case "co":
		return strings.Contains(name, f.target)
	case "sw":
		return strings.HasPrefix(name, f.target)
	case "pr":
		return len(name) > 0
	case "gt":
		return name > f.target
	case "ge":
		return name >= f.target
	case "lt":
		return name < f.target
	case "le":
		return name <= f.target
	default:
		log.Error("invalid name filter:", f)
		return true
	}
}
