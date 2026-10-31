// Licensed under the Universal Permissive License (UPL) Version 1.0.
package etcd

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// A recovery snapshot must cover every page at one committed etcd revision.
// Advancing by the last key plus NUL includes keys with that key as a prefix.
func (s *Store) readSnapshot(parent context.Context) (*pms.PolicyStore, error) {
	ctx, cancel := context.WithTimeout(parent, requestTimeout)
	defer cancel()
	result := &pms.PolicyStore{}
	services := map[string]*pms.Service{}
	committed := map[string]bool{}
	var revision int64
	for _, namespace := range []string{ServicesKey, FunctionsKey} {
		start := s.KeyPrefix + namespace + KeySeparator
		end := clientv3.GetPrefixRangeEnd(start)
		for {
			opts := []clientv3.OpOption{clientv3.WithRange(end), clientv3.WithLimit(pageSize), clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend)}
			if revision != 0 {
				opts = append(opts, clientv3.WithRev(revision))
			}
			page, err := s.client.Get(ctx, start, opts...)
			if err != nil {
				return nil, errors.New(errors.StoreError, "read etcd policy snapshot failed")
			}
			if revision == 0 {
				revision = page.Header.Revision
			}
			for _, kv := range page.Kvs {
				key := string(kv.Key)
				if strings.HasPrefix(key, s.KeyPrefix+FunctionsKey+KeySeparator) {
					var function pms.Function
					if err := json.Unmarshal(kv.Value, &function); err != nil {
						return nil, errors.New(errors.SerializationError, "invalid function in etcd snapshot")
					}
					result.Functions = append(result.Functions, &function)
					continue
				}
				prefix := s.KeyPrefix + ServicesKey + KeySeparator
				if !strings.HasPrefix(key, prefix) {
					continue
				}
				name, part, found := strings.Cut(strings.TrimPrefix(key, prefix), KeySeparator)
				if !found || name == "" {
					continue
				}
				service := services[name]
				if service == nil {
					service = &pms.Service{Name: name}
					services[name] = service
				}
				switch {
				case part == "":
					committed[name] = true
				case part == ServiceTypeKey:
					service.Type = string(kv.Value)
				case strings.HasPrefix(part, PoliciesKey+KeySeparator):
					var policy pms.Policy
					if err := json.Unmarshal(kv.Value, &policy); err != nil {
						return nil, errors.New(errors.SerializationError, "invalid policy in etcd snapshot")
					}
					service.Policies = append(service.Policies, &policy)
				case strings.HasPrefix(part, RolePoliciesKey+KeySeparator):
					var role pms.RolePolicy
					if err := json.Unmarshal(kv.Value, &role); err != nil {
						return nil, errors.New(errors.SerializationError, "invalid role policy in etcd snapshot")
					}
					service.RolePolicies = append(service.RolePolicies, &role)
				}
			}
			if !page.More {
				break
			}
			if len(page.Kvs) == 0 {
				return nil, errors.New(errors.StoreError, "empty continuation page in etcd snapshot")
			}
			start = string(page.Kvs[len(page.Kvs)-1].Key) + "\x00"
		}
	}
	names := make([]string, 0, len(committed))
	for name := range committed {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result.Services = append(result.Services, services[name])
	}
	return result, nil
}
