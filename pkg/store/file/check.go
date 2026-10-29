//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package file

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/eval/condition"
)

// CheckJSON reads a policy store in JSON from r, named name in errors, and
// returns every error in it. Unlike loading, it rejects fields the store does
// not have, since a misspelled field such as "conditon" is otherwise dropped.
func CheckJSON(r io.Reader, name string) (*pms.PolicyStore, []error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, []error{fmt.Errorf("%s: %w", name, err)}
	}
	var ps pms.PolicyStore
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ps); err != nil {
		offset := dec.InputOffset()
		var syntaxErr *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &syntaxErr) {
			// Offset is just past the byte that is wrong.
			offset = max(syntaxErr.Offset-1, 0)
		} else if errors.As(err, &typeErr) {
			offset = typeErr.Offset
		} else if field, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
			// The decoder is past the object by now; point at the field name.
			if i := bytes.LastIndex(data[:offset], []byte(field)); i >= 0 {
				offset = int64(i)
			}
		}
		line, col := lineAndColumn(data, offset)
		return nil, []error{fmt.Errorf("%s:%d:%d: %w", name, line, col, err)}
	}

	var errs []error
	for i, service := range ps.Services {
		where := fmt.Sprintf("%s: service %q", name, service.Name)
		if service.Name == "" {
			where = fmt.Sprintf("%s: service %d", name, i+1)
			errs = append(errs, fmt.Errorf("%s: no name", where))
		}
		for j, policy := range service.Policies {
			for _, problem := range policyProblems(policy) {
				errs = append(errs, fmt.Errorf("%s %s: %s", where, describe("policy", policy.Name, j), problem))
			}
		}
		for j, rolePolicy := range service.RolePolicies {
			for _, problem := range rolePolicyProblems(rolePolicy) {
				errs = append(errs, fmt.Errorf("%s %s: %s", where, describe("role policy", rolePolicy.Name, j), problem))
			}
		}
	}
	return &ps, errs
}

func describe(kind, name string, index int) string {
	if name != "" {
		return fmt.Sprintf("%s %q", kind, name)
	}
	return fmt.Sprintf("%s %d", kind, index+1)
}

func policyProblems(policy *pms.Policy) []string {
	problems := effectProblems(policy.Effect)
	if len(policy.Permissions) == 0 {
		problems = append(problems, "no permissions")
	}
	return append(problems, conditionProblems(policy.Condition)...)
}

func rolePolicyProblems(rolePolicy *pms.RolePolicy) []string {
	problems := effectProblems(rolePolicy.Effect)
	if len(rolePolicy.Roles) == 0 {
		problems = append(problems, "no roles")
	}
	return append(problems, conditionProblems(rolePolicy.Condition)...)
}

// effectProblems reports an effect the evaluator would ignore.
func effectProblems(effect string) []string {
	if effect != pms.Grant && effect != pms.Deny {
		return []string{fmt.Sprintf("effect %q is neither %q nor %q", effect, pms.Grant, pms.Deny)}
	}
	return nil
}

func conditionProblems(cond string) []string {
	if err := condition.Check(cond); err != nil {
		return []string{err.Error()}
	}
	return nil
}

// lineAndColumn returns the 1-based line and column of offset in data.
func lineAndColumn(data []byte, offset int64) (int, int) {
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}
	before := data[:offset]
	line := bytes.Count(before, []byte("\n")) + 1
	col := int(offset) - bytes.LastIndexByte(before, '\n')
	return line, col
}
