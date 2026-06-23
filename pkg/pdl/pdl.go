//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package pdl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/subjectutils"
)

const (
	grant = "grant"
	deny  = "deny"

	resExprPrefix = "expr:"

	// maxPDLInputLength is the maximum allowed length of a PDL input string (1 MiB).
	maxPDLInputLength = 1 << 20
)

// hasPrefixFoldASCII checks whether s starts with prefix using ASCII-only
// case-insensitive comparison. It is a faster replacement for
// strings.EqualFold when both strings are known to be ASCII.
func hasPrefixFoldASCII(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		sc := s[i]
		pc := prefix[i]
		if sc >= 'A' && sc <= 'Z' {
			sc += 'a' - 'A'
		}
		if pc >= 'A' && pc <= 'Z' {
			pc += 'a' - 'A'
		}
		if sc != pc {
			return false
		}
	}
	return true
}

// ParsePolicy parses a line to a policy object
func ParsePolicy(cmd, name string) (*pms.Policy, error) {
	if len(cmd) > maxPDLInputLength {
		return nil, fmt.Errorf("PDL input exceeds maximum length of %d bytes", maxPDLInputLength)
	}
	effect, i, err := getEffect(cmd)
	if err != nil {
		return nil, err
	}
	principals, i, err := getOrPrincipals(cmd, i)
	if err != nil {
		return nil, err
	}
	perms, i, err := getPermissions(cmd, i)
	if err != nil {
		return nil, err
	}
	if len(perms) == 0 {
		return nil, errors.New("No permission found")
	}
	condition, _, err := getCondition(cmd, i)
	if err != nil {
		return nil, err
	}

	policy := pms.Policy{
		Name:        name,
		Effect:      effect,
		Principals:  principals,
		Permissions: perms,
		Condition:   condition,
	}

	return &policy, nil
}

// ParseRolePolicy parses a line to a role policy object
func ParseRolePolicy(cmd, name string) (*pms.RolePolicy, error) {
	if len(cmd) > maxPDLInputLength {
		return nil, fmt.Errorf("PDL input exceeds maximum length of %d bytes", maxPDLInputLength)
	}
	effect, i, err := getEffect(cmd)
	if err != nil {
		return nil, err
	}
	principals, i, err := getRolePolicyPrincipals(cmd, i)
	if err != nil {
		return nil, err
	}
	roles, i, err := getRoles(cmd, i)
	if err != nil {
		return nil, err
	}
	if len(roles) == 0 {
		return nil, errors.New("No role found")
	}
	resources, resExps, i, err := getResources(cmd, i)
	if err != nil {
		return nil, err
	}
	condition, _, err := getCondition(cmd, i)
	if err != nil {
		return nil, err
	}
	rolePolicy := pms.RolePolicy{
		Name:                name,
		Effect:              effect,
		Principals:          principals,
		Resources:           resources,
		ResourceExpressions: resExps,
		Roles:               roles,
		Condition:           condition,
	}
	return &rolePolicy, nil
}

// PolicyToJSON serializes a Policy to JSON and returns it as an io.Reader.
func PolicyToJSON(p *pms.Policy) io.Reader {
	return toJSON(p)
}

// RolePolicyToJSON serializes a RolePolicy to JSON and returns it as an io.Reader.
func RolePolicyToJSON(r *pms.RolePolicy) io.Reader {
	return toJSON(r)
}

func toJSON(i interface{}) io.Reader {
	buf := &bytes.Buffer{}
	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)
	encoder.Encode(i)
	return buf
}

func getEffect(cmd string) (string, int, error) {
	i := skipSpaces(cmd, 0)
	if hasPrefixFoldASCII(cmd[i:], "grant ") {
		return grant, i + 6, nil
	} else if hasPrefixFoldASCII(cmd[i:], "deny ") {
		return deny, i + 5, nil
	} else {
		return "", -1, getError("Not found valid effect", cmd, i)
	}
}

func getRolePolicyPrincipals(cmd string, i int) ([]string, int, error) {
	ret := []string{}
	onePrincipal, i, err := getPrincipal(cmd, i)
	if err != nil {
		return nil, -1, err
	}
	ret = append(ret, onePrincipal)
	i = skipSpaces(cmd, i)
	if i >= len(cmd) {
		return nil, -1, getError("Unexpected EOF found", cmd, i)
	}
	for cmd[i] == ',' {
		// Skip the comma
		i++
		onePrincipal, i, err = getPrincipal(cmd, i)
		if err != nil {
			return nil, -1, err
		}
		ret = append(ret, onePrincipal)
		i = skipSpaces(cmd, i)
		if i >= len(cmd) {
			return nil, -1, getError("Unexpected EOF found", cmd, i)
		}
	}
	return ret, i, nil
}

func getOrPrincipals(cmd string, i int) ([][]string, int, error) {
	ret := [][]string{}
	andPrincipals, i, err := getAndPrincipals(cmd, i)
	if err != nil {
		return nil, -1, err
	}
	ret = append(ret, andPrincipals)
	i = skipSpaces(cmd, i)
	if i >= len(cmd) {
		return nil, -1, getError("Unexpected EOF found", cmd, i)
	}
	for cmd[i] == ',' {
		// Skip the comma
		i++
		andPrincipals, i, err = getAndPrincipals(cmd, i)
		if err != nil {
			return nil, -1, err
		}
		ret = append(ret, andPrincipals)
		i = skipSpaces(cmd, i)
		if i >= len(cmd) {
			return nil, -1, getError("Unexpected EOF found", cmd, i)
		}
	}
	return ret, i, nil
}

func getAndPrincipals(cmd string, i int) ([]string, int, error) {
	// Skip spaces front of and principals first
	i = skipSpaces(cmd, i)
	if i >= len(cmd) {
		return nil, -1, getError("Unexpected EOF found", cmd, i)
	}
	if cmd[i] != '(' {
		// No ( found, only one principal found
		onePrincipal, i, err := getPrincipal(cmd, i)
		return []string{onePrincipal}, i, err
	}

	i++
	if i >= len(cmd) {
		return nil, -1, getError("Unexpected EOF found", cmd, i)
	}

	// principals should be begin with ( and end with )
	i = skipSpaces(cmd, i)
	if i >= len(cmd) {
		return nil, -1, getError("Unexpected EOF found", cmd, i)
	}

	principals := []string{}
	// End of and principals
	var principal string
	var err error
	principal, i, err = getPrincipal(cmd, i)
	if err != nil {
		return nil, -1, err
	}
	if principal == "" {
		// This is an error, there isn't a principal between ()
		return nil, -1, getError("No principal found between ()", cmd, i)
	}
	principals = append(principals, principal)
	i = skipSpaces(cmd, i)
	if i >= len(cmd) {
		return nil, -1, getError("Unexpected EOF found", cmd, i)
	}

	for cmd[i] != ')' {
		i = skipSpaces(cmd, i)
		if i >= len(cmd) {
			return nil, -1, getError("Unexpected EOF found", cmd, i)
		}
		if cmd[i] != ',' {
			return nil, -1, getError("Principals should be sepereated by commas", cmd, i)
		}
		// read ,
		i++
		if i >= len(cmd) {
			return nil, -1, getError("Unexpected EOF found", cmd, i)
		}

		principal, i, err = getPrincipal(cmd, i)
		if err != nil {
			return nil, -1, err
		}
		principals = append(principals, principal)
		i = skipSpaces(cmd, i)
		if i >= len(cmd) {
			return nil, -1, getError("Unexpected EOF found", cmd, i)
		}
	}

	// read ')'
	i++
	return principals, i, nil
}

func getPrincipal(cmd string, i int) (string, int, error) {
	i = skipSpaces(cmd, i)

	var principal ads.Principal
	if hasPrefixFoldASCII(cmd[i:], "user ") {
		i += 5
		principal.Type = ads.PRINCIPAL_TYPE_USER
	} else if hasPrefixFoldASCII(cmd[i:], "group ") {
		i += 6
		principal.Type = ads.PRINCIPAL_TYPE_GROUP
	} else if hasPrefixFoldASCII(cmd[i:], "role ") {
		i += 5
		principal.Type = ads.PRINCIPAL_TYPE_ROLE
	} else if hasPrefixFoldASCII(cmd[i:], "entity ") {
		i += 7
		principal.Type = ads.PRINCIPAL_TYPE_ENTITY
	} else {
		return "", -1, getError("Not found principal type (user|group|role)", cmd, i)
	}

	var err error
	principal.Name, i, err = getToken(cmd, i)
	if err != nil {
		return "", -1, err
	}
	if principal.Name == "" {
		return "", -1, getError("Not found principal name", cmd, i)
	}

	i = skipSpaces(cmd, i)
	if hasPrefixFoldASCII(cmd[i:], "from ") {
		// principal has idd with key word "from"
		i += 5
		i = skipSpaces(cmd, i)
		principal.IDD, i, err = getToken(cmd, i)
		if err != nil {
			return "", -1, err
		}
		if len(principal.IDD) == 0 {
			// no IDD found
			return "", -1, getError("No idd found after key word \"from\"", cmd, i)
		}
	}

	return subjectutils.EncodePrincipal(&principal), i, nil
}

func getRoles(cmd string, i int) ([]string, int, error) {
	tokens := []string{}
	t, i, err := getToken(cmd, i)
	if err != nil {
		return nil, -1, err
	}
	if hasPrefixFoldASCII(t, "role") {
		t, i, err = getToken(cmd, i)
		if err != nil {
			return nil, -1, err
		}
	}
	if t == "" {
		return tokens, i, nil
	}

	tokens = append(tokens, t)
	i = skipSpaces(cmd, i)
	for i < len(cmd) && cmd[i] == ',' {
		t, i, err = getToken(cmd, i+1)
		if err != nil {
			return nil, -1, err
		}
		if hasPrefixFoldASCII(t, "role") {
			t, i, err = getToken(cmd, i)
			if err != nil {
				return nil, -1, err
			}
		}
		if t == "" {
			return nil, -1, getError("Not found role", cmd, i)
		}
		tokens = append(tokens, t)
		i = skipSpaces(cmd, i)
	}
	return tokens, i, nil
}

func getPermissions(cmd string, i int) ([]*pms.Permission, int, error) {
	perms := []*pms.Permission{}
	p, i, err := getPermission(cmd, i)
	if err != nil {
		return nil, -1, err
	}
	if p == nil {
		return perms, i, nil
	}
	perms = append(perms, p)
	i = skipSpaces(cmd, i)
	for i < len(cmd) && cmd[i] == ',' {
		p, i, err = getPermission(cmd, i+1)
		if err != nil {
			return nil, -1, err
		}
		if p == nil {
			return nil, -1, getError("Not found permission", cmd, i)
		}
		perms = append(perms, p)
		i = skipSpaces(cmd, i)
	}
	return perms, i, nil
}

func getPermission(cmd string, i int) (*pms.Permission, int, error) {
	acts, i, err := getTokens(cmd, i, "action")
	if err != nil {
		return nil, i, err
	}
	if len(acts) == 0 {
		return nil, i, getError("Not found permission", cmd, i)
	}
	res, i, err := getToken(cmd, i)
	if err != nil {
		return nil, i, err
	}
	if res == "" {
		return nil, i, getError("Not found permission", cmd, i)
	}

	if isResExpr, resExpr := isResExpr(res); isResExpr {
		return &pms.Permission{ResourceExpression: resExpr, Actions: acts}, i, nil
	}

	return &pms.Permission{Resource: res, Actions: acts}, i, nil
}

func isResExpr(res string) (bool, string) {
	if strings.HasPrefix(res, resExprPrefix) {
		return true, strings.TrimPrefix(res, resExprPrefix)
	}
	return false, res
}

func getResources(cmd string, i int) ([]string, []string, int, error) {
	i = skipSpaces(cmd, i)
	if hasPrefixFoldASCII(cmd[i:], "on ") {
		i += 3
		tokens, i, err := getTokens(cmd, i, "resource")
		if err != nil {
			return nil, nil, i, err
		}
		if len(tokens) == 0 {
			return nil, nil, -1, getError("Not found resource", cmd, i)
		}
		var resources, resExps []string
		for _, token := range tokens {
			isResExpr, resExp := isResExpr(token)
			if !isResExpr {
				resources = append(resources, token)
			} else {
				resExps = append(resExps, resExp)
			}
		}
		return resources, resExps, i, nil
	}
	return nil, []string{}, i, nil
}

func getService(cmd string, i int) (string, int, error) {
	i = skipSpaces(cmd, i)
	if hasPrefixFoldASCII(cmd[i:], "in ") {
		i += 3
		serv, i, err := getToken(cmd, i)
		if err != nil {
			return "", -1, err
		}
		if serv == "" {
			return "", -1, getError("Not found service", cmd, i)
		}
		return serv, i, nil
	}
	return "", i, nil
}

func getCondition(cmd string, i int) (string, int, error) {
	i = skipSpaces(cmd, i)
	if hasPrefixFoldASCII(cmd[i:], "if ") {
		i += 3
		if i >= len(cmd) {
			return "", -1, getError("Unexpected EOF found", cmd, i)
		}
		i = skipSpaces(cmd, i)
		if i >= len(cmd) {
			return "", -1, getError("Unexpected EOF found", cmd, i)
		}
		ret := strings.TrimSpace(cmd[i:])
		i = len(cmd)
		return ret, i, nil
	}
	return "", i, nil
}

func getTokens(cmd string, i int, e string) ([]string, int, error) {
	tokens := []string{}
	t, i, err := getToken(cmd, i)
	if err != nil {
		return nil, -1, err
	}
	if t == "" {
		return tokens, i, nil
	}
	tokens = append(tokens, t)
	i = skipSpaces(cmd, i)
	for i < len(cmd) && cmd[i] == ',' {
		t, i, err = getToken(cmd, i+1)
		if err != nil {
			return nil, -1, err
		}
		if t == "" {
			return nil, -1, getError(fmt.Sprintf("Not found %s", e), cmd, i)
		}
		tokens = append(tokens, t)
		i = skipSpaces(cmd, i)
	}
	return tokens, i, nil
}

func getToken(cmd string, i int) (string, int, error) {
	i = skipSpaces(cmd, i)
	if i >= len(cmd) {
		return "", i, nil
	}
	start := i
	quoted := false
	var end byte
	if cmd[i] == '"' || cmd[i] == '\'' {
		quoted = true
		end = cmd[i]
		i++
		start = i
	}
	for ; i < len(cmd); i++ {
		if quoted {
			if cmd[i] == end {
				return cmd[start:i], i + 1, nil
			}
		} else if cmd[i] == ' ' || cmd[i] == ',' || cmd[i] == '(' || cmd[i] == ')' {
			return cmd[start:i], i, nil
		}
	}
	if quoted {
		return "", i, fmt.Errorf("unclosed quote")
	}
	return cmd[start:i], i, nil
}

func skipSpaces(cmd string, i int) int {
	for i < len(cmd) {
		c := cmd[i]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return i
		}
		i++
	}
	return i
}

func getErrorIndicator(cmd string, pos int) string {
	return fmt.Sprintf("%s\n%s^", cmd, strings.Repeat(" ", pos))
}

func getError(msg, cmd string, pos int) error {
	return fmt.Errorf("%s\n%s", msg, getErrorIndicator(cmd, pos))
}
