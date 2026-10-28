+++
title = "Identity and IAM integration"
description = "Place Speedle+ authorization in an identity and access management (IAM) architecture: trusted identities, identity domains, token asserters and access control."
weight = 40
keywords = ["identity", "IAM", "identity and access management", "authorization", "access control", "Speedle+", "authorization engine", "fine grained access control", "fine-grained access control", "FGA", "policy engine", "authorization policy engine", "externalized authorization", "zero trust"]
bref = "Connect identity to authorization while keeping responsibilities explicit"
+++

## Where Speedle fits in IAM

Identity and access management (IAM) includes establishing identities, authenticating users, managing account lifecycles and controlling access. Speedle+ supplies an authorization engine within that architecture: it evaluates what an identified user, group or service may do. It is not a complete identity directory, login service or account-provisioning system.

| Responsibility | Typical owner |
| --- | --- |
| Accounts, credentials and login | Identity provider and directory |
| Trusted principal names, groups and attributes | Application integration or token asserter |
| Authorization and role policies | Speedle Policy Management Service |
| Access decision | Speedle Authorization Decision Service or embedded evaluator |
| Enforcing the decision on the actual resource | Application or gateway |

## Supply a trusted identity

A trusted application can send explicit principals in the decision request. Alternatively, a configured [token asserter](/docs/assertor/) validates and interprets a token and returns the identities used for policy evaluation. Implement and configure the asserter for the actual identity provider; the existence of the webhook does not establish compatibility with every token format or protocol.

Do not expose an authorization endpoint that lets an untrusted caller impersonate another user by simply naming that user. Authenticate callers, derive identity and attributes from trusted sources, and protect decision and management services according to the [security guidance](/docs/security/).

## Distinguish identity domains

The same user name may exist in multiple domains. Try this policy in the [Playground](/playground/):

```text
grant user alice from workforce read book
```

Use user `alice`, identity domain `workforce`, resource `book` and action `read`: access is allowed. Change only the identity domain to `partner`: access is denied. The domain string is an explicit namespace in the request, not proof that authentication occurred.

The equivalent Playground request is:

```json
{
  "subject": {"principals": [{"type": "user", "name": "alice", "idd": "workforce"}]},
  "resource": "book",
  "action": "read"
}
```

Omitting `from workforce` from the policy would allow the named user to match across identity domains. Review that choice carefully in multitenant integrations. The browser demo accepts explicit principals for experimentation; it does not authenticate users or invoke the token asserter.

See [identity domains](/docs/idd/), [ABAC attributes](/docs/access-control/abac/) and the [token assertion API](/docs/api/asserter_api/) for implementation details.
