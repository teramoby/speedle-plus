package authz

import rego.v1

default allow := false

roles contains role if {
	some role in graph.reachable(data.role_inherits, data.user_roles[input.user])
}

allow if {
	some role in roles
	some perm in data.role_perms[role]
	perm.action == input.action
	perm.resource == input.resource
}
