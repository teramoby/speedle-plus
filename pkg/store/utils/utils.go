//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package utils

import (
	"encoding/json"
	"io"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/suid"
)

func ReadFilePolicyStore(policyStoreFile string) (*pms.PolicyStore, error) {
	file, err := os.Open(policyStoreFile)
	if err != nil {
		return nil, errors.Wrapf(err, errors.StoreError, "unable to open file %q", policyStoreFile)
	}
	defer file.Close()
	ret, err := readPolicyStore(file)
	return ret, err
}

func readPolicyStore(reader io.Reader) (*pms.PolicyStore, error) {
	decoder := json.NewDecoder(reader)
	var policyStore pms.PolicyStore
	if err := decoder.Decode(&policyStore); err != nil {
		return nil, errors.Wrap(err, errors.SerializationError, "unable to decode policy store")
	}
	return &policyStore, nil
}

// ValidateFunc validates a function definition.
func ValidateFunc(function *pms.Function) error {
	if function.Name == "" || function.FuncURL == "" {
		return errors.New(errors.InvalidRequest, "\"name\" and \"funcURL\" in function definition can not be empty")
	}
	// Validate the function URL at registration time so that a malicious
	// FuncURL pointing at an internal/private address (SSRF) is rejected
	// before it is ever stored and evaluated.
	return ValidateFunctionURL(function.FuncURL)
}

// ValidateFunctionURL checks that a custom function URL is safe to call: it must
// use http/https and must not resolve to a private, loopback, link-local, or
// unspecified address (SSRF protection). localhost is permitted for local
// development.
func ValidateFunctionURL(urlStr string) error {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return errors.Wrapf(err, errors.InvalidRequest, "invalid function URL %q", urlStr)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.Errorf(errors.InvalidRequest, "unsupported URL scheme %q in function URL %q", parsed.Scheme, urlStr)
	}
	host := parsed.Hostname() // strips brackets from IPv6 and the port
	// Allow localhost for local development/testing.
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return errors.Errorf(errors.InvalidRequest, "function URL %q resolves to a private/internal address, which is not allowed", urlStr)
		}
	}
	return nil
}

// Filter represents a parsed filter expression.
type Filter struct {
	Field    string
	Operator string
	Target   string
}

func (f *Filter) String() string {
	return f.Field + " " + f.Operator + " " + f.Target
}

// ParseFilter parses a filter string in the format "field operator [target]".
func ParseFilter(filterStr string) *Filter {
	if len(filterStr) == 0 {
		return nil
	}
	if !strings.HasPrefix(filterStr, "name") {
		return nil
	}
	values := strings.Split(filterStr, " ")
	if len(values) == 2 {
		return &Filter{Field: values[0], Operator: values[1]}
	}
	if len(values) == 3 {
		return &Filter{Field: values[0], Operator: values[1], Target: values[2]}
	}
	return nil
}

// NameFilter returns true if the name passes the given filter.
func NameFilter(name string, f *Filter) bool {
	if f == nil {
		return true
	}
	switch f.Operator {
	case "eq":
		return name == f.Target
	case "co":
		return strings.Contains(name, f.Target)
	case "sw":
		return strings.HasPrefix(name, f.Target)
	case "pr":
		return true
	default:
		return true
	}
}

// GenerateID deep-copies a service and assigns new IDs to all policies and role policies.
func GenerateID(service *pms.Service) *pms.Service {
	result := *service
	result.Policies = make([]*pms.Policy, len(service.Policies))
	for i, p := range service.Policies {
		cp := *p
		cp.ID = suid.New().String()
		result.Policies[i] = &cp
	}
	result.RolePolicies = make([]*pms.RolePolicy, len(service.RolePolicies))
	for i, rp := range service.RolePolicies {
		crp := *rp
		crp.ID = suid.New().String()
		result.RolePolicies[i] = &crp
	}
	if result.Policies == nil {
		result.Policies = []*pms.Policy{}
	}
	if result.RolePolicies == nil {
		result.RolePolicies = []*pms.RolePolicy{}
	}
	return &result
}
