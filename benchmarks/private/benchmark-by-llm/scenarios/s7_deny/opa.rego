package authz

import rego.v1

default allow := false

allow if {
	some role in data.user_roles[input.user]
	some grant in data.grants[role]
	grant.action == input.action
	glob.match(grant.resource, ["/"], input.resource)
	not denied
}

denied if {
	some d in data.denies[input.user]
	d.action == input.action
	glob.match(d.resource, ["/"], input.resource)
}
