package authz

import rego.v1

default allow := false

allow if {
	input.action == "approve"
	input.resource == "/expense"
	some role in data.user_roles[input.user]
	input.attrs.amount <= data.approval_limit[role]
}
