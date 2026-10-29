package authz

import rego.v1

default allow := false

allow if {
	some role in data.tenant_roles[input.tenant][input.user]
	some perm in data.role_perms[role]
	perm.action == input.action
	perm.resource == input.resource
}
