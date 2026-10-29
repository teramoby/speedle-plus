---
name: speedle-policy-manager
description: Speedle Policy Manager. Turns an organization's business processes and management rules into a clear, least-privilege authorization model (RBAC/ABAC) and expresses it in SPDL, producing a policy file Speedle+ can load directly plus test cases.
---

# Speedle Policy Manager

Goal: take the business description, org structure and management rules the user provides (approval authority matrices, separation-of-duties requirements, data access rules, and so on) and turn them into an authorization model that is **traceable, least-privilege and easy to maintain**, expressed in SPDL (Speedle / Speedle+ Policy Definition Language).

## Workflow

### 1. Gather input and fill gaps
Extract the elements below from the user's material. Where something is missing and would change the policies, ask all the questions at once (3–5 at most); for the rest, make reasonable assumptions and list them in the output.

- **Subjects**: people (`user`), organizations, departments or positions (`group`), systems or service accounts (`entity`)
- **Resources**: business objects and their hierarchy (e.g. company / department / document), and which must be isolated by department, project or tenant
- **Actions**: the business operations on each resource type (create / read / update / approve / pay / export …). Use business verbs rather than flattening everything to CRUD.
- **Rules**: number each one (R1, R2 …), including amount or level thresholds, time windows, ownership restrictions, separation of duties (SoD) and explicit prohibitions
- **Runtime attributes**: the data each rule depends on (amount, document owner, status, classification …), and whether the application can pass it into the authorization request from a trusted source

### 2. Design the model
Design the model first, then write SPDL, following these principles:

1. **Roles first**: grant permissions to roles; people and groups get roles through role policies. Avoid granting business permissions directly to individuals.
2. **Map the organization with groups**: departments and positions coming from the IdP are `group`s mapped to business roles (`grant group Finance Accountant`).
3. **Use role hierarchy to avoid duplication**: when a senior role includes a junior role's abilities, use `grant role Manager Employee` instead of repeating grants.
4. **Hierarchical resource paths**: design them as `/domain/isolation-dimension/object-id` (e.g. `/expense/{dept}/{id}`) so isolation can be expressed with `expr:` regexes and resource-scoped roles.
5. **Scope with `on`**: express "only within my department / project" with a resource-scoped role: `grant user zhangsan DeptManager on expr:^/expense/sales/.*$`.
6. **Conditions for ABAC**: amount thresholds, ownership, status and time go into `if` conditions.
7. **Use deny only for unconditional hard prohibitions** (blocklists, contractors may never pay, etc.). **Prohibitions that depend on attributes — such as the SoD rule "nobody approves their own document" — go into the grant's condition**: `... approve ... if amount <= 5000 && owner != request_user`. See pitfall 1 below for why.
8. **Least privilege**: grant only the actions the rules explicitly allow. Anything not granted is denied, so no catch-all deny is needed.

### 3. Write the SPDL and self-check
Write a complete `.spdl` file using the syntax reference below, with a `#` comment above each policy naming the rule it implements. Then go through the self-check list.

### 4. Output
Produce the following sections:

1. **Model overview**: a roles table (role / meaning / inherits / assigned to), a resources table (path pattern / description), an actions table, and an attributes table (name / type / source / who passes it)
2. **Rule → policy traceability table**: rule ID | rule summary | SPDL policy | notes
3. **Complete SPDL file** (ready to save as `policies.spdl` and load)
4. **Test case table**: subject | resource | action | attributes | expected result | matching policy / reason. Cover the allow, deny and boundary cases of every rule (e.g. an amount exactly at the threshold), separation of duties, and cross-scope access attempts.
5. **Assumptions and open questions**

If Go and speedle-plus are available, run the test cases against the real engine with `eval.NewFromFile("policies.spdl", false)` and `IsAllowed`.

---

## SPDL syntax reference

### File structure
```
[service.<service-name>]
[policy]
# authorization policies, one per line
[rolepolicy]
# role policies, one per line
```
A file may contain several `[service.x]` sections. Lines starting with `#` are comments. Policies apply only within their service.

### Authorization policy
```
EFFECT SUBJECT ACTION[,ACTION]* RESOURCE [if CONDITION]
EFFECT    = grant | deny
SUBJECT   = PRINCIPAL | (PRINCIPAL, PRINCIPAL, ...)   comma-separated principals are OR; principals inside parentheses are AND
PRINCIPAL = user|group|entity|role <name> [from <identity-domain>]
```
Examples:
```
grant role Employee create expr:^/expense/[^/]+$
grant group Administrators list,watch,get expr:^c1/default/core/pods/.*$
grant (role designer, role dba) update db_design_doc     # both roles are required
deny group Contractors pay expr:^/expense/.*$
```

### Role policy
```
EFFECT SUBJECT [role] <role-name> [on RESOURCE] [if CONDITION]
```
Examples:
```
grant group AllStaff Employee
grant role DeptManager Employee                          # role inheritance: a manager is also an employee
grant user zhangsan DeptManager on expr:^/expense/sales/.*$   # manager only on sales resources
grant user Tony PartTimeEmployee if request_hour > 22 || request_hour < 8
```
Note: assigning a role grants nothing by itself; there must also be an authorization policy for that role.

### Resources
- Exact match: `/books/HarryPotter`
- Regex match: `expr:<Go regular expression>`. Not shell globs — use `.*`, and anchor with `^` and `$` when the whole identifier must match.

### Conditions (`if`)
- Constants: strings in **single quotes** `'approved'`; numbers `5000`; booleans `true/false`; datetimes in RFC 3339 `'2026-01-02T15:04:05+08:00'`; arrays `('a','b')`
- Comparators: `== != > >= < <=`; string regex `=~`; membership `in` (`status in ('a','b')`, `'manager' in request_groups`)
- Logic: `&& || !`; arithmetic `+ - * / %`; string concatenation `+`. Comparisons bind tighter than `&&`, and `&&` tighter than `||`; parenthesize complex conditions.
- Built-in attributes: `request_user` `request_groups` (array) `request_entity` `request_resource` `request_action` `request_time` `request_year` `request_month` `request_day` `request_hour` (0–23) `request_weekday` ('Monday' …)
- Built-in functions: `Sqrt` `Max` `Min` `Sum` `Avg` `IsSubSet(a, b)`; function names are case-sensitive
- Custom attributes such as `amount`, `owner`, `status` are passed by the application in the authorization request (REST types are limited to string / numeric / bool / datetime). Name them `[A-Za-z][A-Za-z0-9_]*` and avoid clashing with built-in names.

### Decision rules
- Deny overrides grant; with no applicable grant the request is denied (default deny)
- Reason codes: GRANT_POLICY_FOUND / DENY_POLICY_FOUND / NO_APPLICABLE_POLICIES, etc.
- Keywords are case-insensitive; principal, action and resource names are case-sensitive

---

## Key pitfalls (verified on the Speedle+ engine)

1. **If an attribute used in a condition is missing, that policy simply does not apply** (the engine logs an error and skips it). For a grant this is safe (fail closed), but **a deny silently stops working**: `deny role Employee approve ... if owner == request_user` blocks nothing when the application omits `owner`. So write attribute-dependent prohibitions as a negated condition on the grant (`owner != request_user`), and reserve deny for unconditional prohibitions.
2. Split permissions for the same subject when actions need different conditions. For example, "a manager may read regardless of amount but approve only ≤ 5000" needs two policies; don't put `read,approve` in one policy with an amount condition.
3. An `expr:` without `^...$` may match resources with unexpected prefixes or suffixes.
4. Double-quoted or unquoted string constants are a common mistake; always use single quotes.
5. The application must take attributes from trusted sources (database, token), never from request parameters the user can tamper with. State the source in the attributes table.
6. SPDL is stateless: a rule like "the submitter may not approve" relies on the application passing `owner` / `submitter`. Workflow controls such as "two-person sign-off" belong in the workflow engine, not the authorization policy; say so in the output.

## Self-check list
- [ ] Every rule has a corresponding policy, or an explanation of why it is not enforced at the authorization layer
- [ ] No action is granted that the rules don't allow; no business permission is granted directly to an individual (unless the rules require it)
- [ ] Department / project / tenant isolation is enforced by resource paths plus `on` or conditions, and cross-scope requests are denied
- [ ] Every deny is unconditional, or its attributes are guaranteed to be passed
- [ ] Every custom attribute is listed in the attributes table with its type and source
- [ ] Regexes are anchored and string constants use single quotes
- [ ] Test cases cover allow, deny, boundary and privilege-escalation scenarios, and the expected results match a walk-through of the policies

---

## Complete example: expense reimbursement

**Rules**: R1 employees may submit expenses in their own department; R2 employees may read / update only their own reports; R3 department managers may read all reports in their department; R4 department managers approve their department's reports of ≤ 5000; R5 reports > 5000 are approved by the finance director; R6 nobody may approve their own report; R7 accountants may pay only approved reports; R8 contractors may never pay.

```
[service.expense]
[policy]
# R1 (create targets the department collection /expense/{dept})
grant role Employee create expr:^/expense/[^/]+$ if request_resource == '/expense/' + requester_dept
# R2
grant role Employee read,update expr:^/expense/[^/]+/[^/]+$ if owner == request_user
# R3 (scope limited by the `on` in the role policy)
grant role DeptManager read expr:^/expense/[^/]+/[^/]+$
# R4 + R6
grant role DeptManager approve expr:^/expense/[^/]+/[^/]+$ if amount <= 5000 && owner != request_user
# R5 + R6
grant role FinanceDirector read expr:^/expense/[^/]+/[^/]+$
grant role FinanceDirector approve expr:^/expense/[^/]+/[^/]+$ if amount > 5000 && owner != request_user
# R7
grant role Accountant pay expr:^/expense/[^/]+/[^/]+$ if status == 'approved'
# R8
deny group Contractors pay expr:^/expense/.*$
[rolepolicy]
grant group AllStaff Employee
grant group Finance Accountant
grant user lisi FinanceDirector
grant user zhangsan DeptManager on expr:^/expense/sales/.*$
```

Resource paths: `/expense/{dept}` for creating a report, `/expense/{dept}/{reportId}` for an existing report. Attributes: `requester_dept` (string, the requester's department, from the HR directory), `owner` (string, report submitter, from the database), `amount` (numeric), `status` (string).

| Subject | Resource | Action | Attributes | Result |
|---|---|---|---|---|
| wang (AllStaff) | /expense/sales | create | requester_dept=sales | Allow |
| wang | /expense/rd | create | requester_dept=sales | Deny (R1, other department) |
| wang | /expense/sales | create | requester_dept not passed | Deny (fail closed) |
| zhangsan (AllStaff) | /expense/sales/r2 | approve | owner=zhao, amount=3000 | Allow |
| zhangsan | /expense/sales/r3 | approve | owner=zhangsan, amount=3000 | Deny (R6) |
| zhangsan | /expense/sales/r3 | approve | owner not passed | Deny (fail closed) |
| zhangsan | /expense/rd/r4 | approve | owner=zhao, amount=3000 | Deny (not his department) |
| zhangsan | /expense/sales/r5 | approve | owner=zhao, amount=8000 | Deny (over limit) |
| lisi | /expense/rd/r5 | approve | owner=zhao, amount=8000 | Allow |
| qian (Finance) | /expense/sales/r5 | pay | status=approved | Allow |
| sun (Finance, Contractors) | /expense/sales/r5 | pay | status=approved | Deny (R8 deny) |
