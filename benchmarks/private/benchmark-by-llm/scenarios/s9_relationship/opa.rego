package authz

import rego.v1

default allow := false

allow if {
	some node in graph.reachable(data.parent, {input.resource})
	input.action in data.grants[input.user][node]
}
