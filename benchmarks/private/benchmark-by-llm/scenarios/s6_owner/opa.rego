package authz

import rego.v1

default allow := false

allow if {
	input.action == "edit"
	input.attrs.owner == input.user
}
