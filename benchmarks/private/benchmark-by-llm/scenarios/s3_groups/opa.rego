package authz

import rego.v1

default allow := false

allow if {
	some group in input.groups
	some role in data.group_roles[group]
	some perm in data.role_perms[role]
	perm.action == input.action
	perm.resource == input.resource
}
