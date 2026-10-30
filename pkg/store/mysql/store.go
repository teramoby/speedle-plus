// Licensed under the Universal Permissive License (UPL) Version 1.0.
package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	stderrors "errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	driver "github.com/go-sql-driver/mysql"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
	storeutils "github.com/teramoby/speedle-plus/pkg/store/utils"
	"github.com/teramoby/speedle-plus/pkg/suid"
)

const (
	serviceColumns  = "name,service_type,metadata"
	policyColumns   = "id,name,effect,permissions,principals,condition_expr,metadata"
	roleColumns     = "id,name,effect,roles,principals,resources,resource_expressions,condition_expr,metadata"
	functionColumns = "name,description,func_url,local_func_url,ca,result_cachable,result_ttl,metadata"
	revisionSQL     = "SELECT revision FROM speedle_store_meta WHERE id = 1"
)

// Store implements the full PMS interface. One database is one policy store.
// Close must be called by embedded users after stopping their evaluator.
type Store struct {
	db                    *sql.DB
	timeout, pollInterval time.Duration
	secrets               []string
	mu                    sync.Mutex
	closed                bool
	watch                 *watcher
}

var _ pms.PolicyStoreManager = (*Store)(nil)

func (*Store) Type() string { return StoreType }

func (s *Store) translate(err error) error {
	if err == nil {
		return nil
	}
	if errors.Code(err) != errors.UnknownError {
		return err
	}
	code := errors.StoreError
	var mysqlErr *driver.MySQLError
	if stderrors.Is(err, sql.ErrNoRows) {
		code = errors.EntityNotFound
	}
	if stderrors.As(err, &mysqlErr) {
		switch mysqlErr.Number {
		case 1062:
			code = errors.EntityAlreadyExists
		case 1452:
			code = errors.EntityNotFound
		case 1406, 1366:
			code = errors.InvalidRequest
		}
	}
	msg := err.Error()
	for _, secret := range s.secrets {
		if secret != "" {
			msg = strings.ReplaceAll(msg, secret, "[redacted]")
		}
	}
	// Do not retain an unredacted cause, which can be logged separately by callers.
	return errors.New(code, "MySQL: "+msg)
}

func (s *Store) transaction(parent context.Context, write bool, fn func(context.Context, *sql.Tx) error) error {
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: !write})
	if err != nil {
		return s.translate(err)
	}
	defer tx.Rollback()
	if write {
		var revision int64
		if err = tx.QueryRowContext(ctx, revisionSQL+" FOR UPDATE").Scan(&revision); err != nil {
			return s.translate(err)
		}
	}
	if err = fn(ctx, tx); err != nil {
		return s.translate(err)
	}
	if write {
		result, err := tx.ExecContext(ctx, "UPDATE speedle_store_meta SET revision = revision + 1 WHERE id = 1 AND revision < 9223372036854775807")
		if err != nil {
			return s.translate(err)
		}
		n, err := result.RowsAffected()
		if err != nil {
			return s.translate(err)
		}
		if n != 1 {
			return errors.New(errors.StoreError, "MySQL revision is exhausted or missing")
		}
	}
	return s.translate(tx.Commit())
}

func (s *Store) write(fn func(context.Context, *sql.Tx) error) error {
	return s.transaction(context.Background(), true, fn)
}

func validKey(value string) error {
	if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > 255 {
		return errors.New(errors.InvalidRequest, "MySQL names and IDs must contain 1 to 255 valid Unicode characters")
	}
	return nil
}

// All callers pass fixed PMS field types (strings, arrays and maps), which are
// always JSON-serializable. JSON null deliberately preserves nil slices/maps.
func jsonText(value interface{}) string { b, _ := json.Marshal(value); return string(b) }

type jsonTarget[T any] struct{ value *T }

func (j jsonTarget[T]) Scan(src interface{}) error {
	var raw []byte
	switch v := src.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return errors.New(errors.SerializationError, "invalid MySQL JSON field")
	}
	if err := json.Unmarshal(raw, j.value); err != nil {
		return errors.New(errors.SerializationError, "invalid MySQL JSON data")
	}
	return nil
}
func jsonInto[T any](target *T) jsonTarget[T] { return jsonTarget[T]{target} }

func query[T any](ctx context.Context, tx *sql.Tx, sqlText string, args []interface{}, scan func(*sql.Rows) (*T, error)) ([]*T, error) {
	rows, err := tx.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]*T, 0)
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func scanService(rows *sql.Rows) (*pms.Service, error) {
	v := &pms.Service{}
	err := rows.Scan(&v.Name, &v.Type, jsonInto(&v.Metadata))
	return v, err
}
func scanPolicy(rows *sql.Rows) (*pms.Policy, error) {
	v := &pms.Policy{}
	err := rows.Scan(&v.ID, &v.Name, &v.Effect, jsonInto(&v.Permissions), jsonInto(&v.Principals), &v.Condition, jsonInto(&v.Metadata))
	return v, err
}
func scanRole(rows *sql.Rows) (*pms.RolePolicy, error) {
	v := &pms.RolePolicy{}
	err := rows.Scan(&v.ID, &v.Name, &v.Effect, jsonInto(&v.Roles), jsonInto(&v.Principals), jsonInto(&v.Resources), jsonInto(&v.ResourceExpressions), &v.Condition, jsonInto(&v.Metadata))
	return v, err
}
func scanFunction(rows *sql.Rows) (*pms.Function, error) {
	v := &pms.Function{}
	err := rows.Scan(&v.Name, &v.Description, &v.FuncURL, &v.LocalFuncURL, &v.CA, &v.ResultCachable, &v.ResultTTL, jsonInto(&v.Metadata))
	return v, err
}

func policies(ctx context.Context, tx *sql.Tx, name string) ([]*pms.Policy, error) {
	return query(ctx, tx, "SELECT "+policyColumns+" FROM speedle_policies WHERE service_name = ? ORDER BY id", []interface{}{name}, scanPolicy)
}
func roles(ctx context.Context, tx *sql.Tx, name string) ([]*pms.RolePolicy, error) {
	return query(ctx, tx, "SELECT "+roleColumns+" FROM speedle_role_policies WHERE service_name = ? ORDER BY id", []interface{}{name}, scanRole)
}
func requireService(ctx context.Context, tx *sql.Tx, name string) error {
	var found string
	return tx.QueryRowContext(ctx, "SELECT name FROM speedle_services WHERE name = ?", name).Scan(&found)
}
func services(ctx context.Context, tx *sql.Tx, name string) ([]*pms.Service, error) {
	clause := " ORDER BY name"
	var args []interface{}
	if name != "" {
		clause = " WHERE name = ?"
		args = []interface{}{name}
	}
	list, err := query(ctx, tx, "SELECT "+serviceColumns+" FROM speedle_services"+clause, args, scanService)
	if err != nil {
		return nil, err
	}
	for _, service := range list {
		service.Policies, err = policies(ctx, tx, service.Name)
		if err != nil {
			return nil, err
		}
		service.RolePolicies, err = roles(ctx, tx, service.Name)
		if err != nil {
			return nil, err
		}
	}
	return list, nil
}

func (s *Store) snapshot(ctx context.Context, after *int64) (*pms.PolicyStore, int64, error) {
	var ps *pms.PolicyStore
	var revision int64
	err := s.transaction(ctx, false, func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, revisionSQL).Scan(&revision); err != nil {
			return err
		}
		if after != nil && revision == *after {
			return nil
		}
		result := &pms.PolicyStore{}
		var err error
		result.Services, err = services(ctx, tx, "")
		if err != nil {
			return err
		}
		result.Functions, err = query(ctx, tx, "SELECT "+functionColumns+" FROM speedle_functions ORDER BY name", nil, scanFunction)
		if err != nil {
			return err
		}
		ps = result
		return nil
	})
	return ps, revision, err
}

func (s *Store) ReadPolicyStore() (*pms.PolicyStore, error) {
	ps, _, err := s.snapshot(context.Background(), nil)
	return ps, err
}

func insertPolicy(ctx context.Context, tx *sql.Tx, name string, p *pms.Policy, preserve bool) (*pms.Policy, error) {
	if p == nil {
		return nil, errors.New(errors.InvalidRequest, "policy is required")
	}
	v := *p
	if !preserve || v.ID == "" {
		v.ID = suid.New().String()
	}
	if err := validKey(v.ID); err != nil {
		return nil, err
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO speedle_policies (service_name,"+policyColumns+") VALUES (?,?,?,?,?,?,?,?)",
		name, v.ID, v.Name, v.Effect, jsonText(v.Permissions), jsonText(v.Principals), v.Condition, jsonText(v.Metadata))
	return &v, err
}
func insertRole(ctx context.Context, tx *sql.Tx, name string, p *pms.RolePolicy, preserve bool) (*pms.RolePolicy, error) {
	if p == nil {
		return nil, errors.New(errors.InvalidRequest, "role policy is required")
	}
	v := *p
	if !preserve || v.ID == "" {
		v.ID = suid.New().String()
	}
	if err := validKey(v.ID); err != nil {
		return nil, err
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO speedle_role_policies (service_name,"+roleColumns+") VALUES (?,?,?,?,?,?,?,?,?,?)",
		name, v.ID, v.Name, v.Effect, jsonText(v.Roles), jsonText(v.Principals), jsonText(v.Resources), jsonText(v.ResourceExpressions), v.Condition, jsonText(v.Metadata))
	return &v, err
}
func insertService(ctx context.Context, tx *sql.Tx, v *pms.Service, preserve bool) error {
	if v == nil {
		return errors.New(errors.InvalidRequest, "service is required")
	}
	if err := validKey(v.Name); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO speedle_services ("+serviceColumns+") VALUES (?,?,?)", v.Name, v.Type, jsonText(v.Metadata)); err != nil {
		return err
	}
	for _, p := range v.Policies {
		if _, err := insertPolicy(ctx, tx, v.Name, p, preserve); err != nil {
			return err
		}
	}
	for _, p := range v.RolePolicies {
		if _, err := insertRole(ctx, tx, v.Name, p, preserve); err != nil {
			return err
		}
	}
	return nil
}
func insertFunction(ctx context.Context, tx *sql.Tx, v *pms.Function) error {
	if v == nil || v.FuncURL == "" {
		return errors.New(errors.InvalidRequest, "function name and funcURL are required")
	}
	if err := validKey(v.Name); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO speedle_functions ("+functionColumns+") VALUES (?,?,?,?,?,?,?,?)",
		v.Name, v.Description, v.FuncURL, v.LocalFuncURL, v.CA, v.ResultCachable, v.ResultTTL, jsonText(v.Metadata))
	return err
}

func (s *Store) WritePolicyStore(ps *pms.PolicyStore) error {
	ps, err := storeutils.PreparePolicyStore(ps)
	if err != nil {
		return err
	}
	return s.write(func(ctx context.Context, tx *sql.Tx) error {
		for _, table := range []string{"speedle_services", "speedle_functions"} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
				return err
			}
		}
		for _, v := range ps.Services {
			if err := insertService(ctx, tx, v, true); err != nil {
				return err
			}
		}
		for _, v := range ps.Functions {
			if err := insertFunction(ctx, tx, v); err != nil {
				return err
			}
		}
		return nil
	})
}
func (s *Store) CreateService(v *pms.Service) error {
	if err := storeutils.ValidateServiceRolePolicies(v); err != nil {
		return err
	}
	prepared := *v
	prepared.Policies = make([]*pms.Policy, len(v.Policies))
	for i, policy := range v.Policies {
		if policy == nil {
			return errors.New(errors.InvalidRequest, "policy is required")
		}
		copy := *policy
		copy.ID = suid.New().String()
		prepared.Policies[i] = &copy
	}
	prepared.RolePolicies = make([]*pms.RolePolicy, len(v.RolePolicies))
	for i, policy := range v.RolePolicies {
		copy := *policy
		copy.ID = suid.New().String()
		prepared.RolePolicies[i] = &copy
	}
	err := s.write(func(ctx context.Context, tx *sql.Tx) error { return insertService(ctx, tx, &prepared, true) })
	// Return generated IDs to PMS responses only after the transaction commits.
	if err == nil {
		*v = prepared
	}
	return err
}
func (s *Store) CreatePolicy(name string, p *pms.Policy) (*pms.Policy, error) {
	var result *pms.Policy
	err := s.write(func(ctx context.Context, tx *sql.Tx) error {
		if err := requireService(ctx, tx, name); err != nil {
			return err
		}
		var err error
		result, err = insertPolicy(ctx, tx, name, p, false)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
func (s *Store) CreateRolePolicy(name string, p *pms.RolePolicy) (*pms.RolePolicy, error) {
	if err := storeutils.ValidateRolePolicyPrincipals(p); err != nil {
		return nil, err
	}
	var result *pms.RolePolicy
	err := s.write(func(ctx context.Context, tx *sql.Tx) error {
		if err := requireService(ctx, tx, name); err != nil {
			return err
		}
		var err error
		result, err = insertRole(ctx, tx, name, p, false)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
func (s *Store) CreateFunction(v *pms.Function) (*pms.Function, error) {
	err := s.write(func(ctx context.Context, tx *sql.Tx) error { return insertFunction(ctx, tx, v) })
	if err != nil {
		return nil, err
	}
	copy := *v
	return &copy, nil
}

func (s *Store) ListAllServices() ([]*pms.Service, error) {
	var result []*pms.Service
	err := s.transaction(context.Background(), false, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		result, err = services(ctx, tx, "")
		return err
	})
	return result, err
}
func (s *Store) GetService(name string) (*pms.Service, error) {
	if err := validKey(name); err != nil {
		return nil, err
	}
	var result []*pms.Service
	err := s.transaction(context.Background(), false, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		result, err = services(ctx, tx, name)
		if err == nil && len(result) == 0 {
			return sql.ErrNoRows
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return result[0], nil
}
func (s *Store) GetServiceNames() ([]string, error) {
	result := make([]string, 0)
	err := s.transaction(context.Background(), false, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT name FROM speedle_services ORDER BY name")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return err
			}
			result = append(result, name)
		}
		return rows.Err()
	})
	return result, err
}

// Filtering matches the file/etcd name-filter contract, including its
// permissive handling of unsupported filters. No user input becomes SQL.
func nameMatches(name, filter string) bool {
	if !strings.HasPrefix(filter, "name") {
		return true
	}
	fields := strings.Split(filter, " ")
	if len(fields) < 2 {
		return true
	}
	target := ""
	if len(fields) > 2 {
		target = fields[2]
	}
	if fields[0] != "name" {
		return true
	}
	switch fields[1] {
	case "eq":
		return name == target
	case "co":
		return strings.Contains(name, target)
	case "sw":
		return strings.HasPrefix(name, target)
	case "pr":
		return name != ""
	case "gt":
		return name > target
	case "ge":
		return name >= target
	case "lt":
		return name < target
	case "le":
		return name <= target
	default:
		return true
	}
}

func (s *Store) ListAllPolicies(name, filter string) ([]*pms.Policy, error) {
	var result []*pms.Policy
	err := s.transaction(context.Background(), false, func(ctx context.Context, tx *sql.Tx) error {
		if err := requireService(ctx, tx, name); err != nil {
			return err
		}
		list, err := policies(ctx, tx, name)
		if err != nil {
			return err
		}
		result = make([]*pms.Policy, 0)
		for _, p := range list {
			if nameMatches(p.Name, filter) {
				result = append(result, p)
			}
		}
		return nil
	})
	return result, err
}
func (s *Store) ListAllRolePolicies(name, filter string) ([]*pms.RolePolicy, error) {
	var result []*pms.RolePolicy
	err := s.transaction(context.Background(), false, func(ctx context.Context, tx *sql.Tx) error {
		if err := requireService(ctx, tx, name); err != nil {
			return err
		}
		list, err := roles(ctx, tx, name)
		if err != nil {
			return err
		}
		result = make([]*pms.RolePolicy, 0)
		for _, p := range list {
			if nameMatches(p.Name, filter) {
				result = append(result, p)
			}
		}
		return nil
	})
	return result, err
}
func (s *Store) ListAllFunctions(filter string) ([]*pms.Function, error) {
	var result []*pms.Function
	err := s.transaction(context.Background(), false, func(ctx context.Context, tx *sql.Tx) error {
		list, err := query(ctx, tx, "SELECT "+functionColumns+" FROM speedle_functions ORDER BY name", nil, scanFunction)
		if err != nil {
			return err
		}
		result = make([]*pms.Function, 0)
		for _, v := range list {
			if nameMatches(v.Name, filter) {
				result = append(result, v)
			}
		}
		return nil
	})
	return result, err
}

func get[T any](s *Store, table, columns, where string, args []interface{}, scan func(*sql.Rows) (*T, error)) (*T, error) {
	var result []*T
	err := s.transaction(context.Background(), false, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		result, err = query(ctx, tx, "SELECT "+columns+" FROM "+table+" WHERE "+where, args, scan)
		if err == nil && len(result) == 0 {
			return sql.ErrNoRows
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return result[0], nil
}
func (s *Store) GetPolicy(name, id string) (*pms.Policy, error) {
	return get(s, "speedle_policies", policyColumns, "service_name = ? AND id = ?", []interface{}{name, id}, scanPolicy)
}
func (s *Store) GetRolePolicy(name, id string) (*pms.RolePolicy, error) {
	return get(s, "speedle_role_policies", roleColumns, "service_name = ? AND id = ?", []interface{}{name, id}, scanRole)
}
func (s *Store) GetFunction(name string) (*pms.Function, error) {
	return get(s, "speedle_functions", functionColumns, "name = ?", []interface{}{name}, scanFunction)
}

func (s *Store) count(table, name string) (int64, error) {
	var n int64
	err := s.transaction(context.Background(), false, func(ctx context.Context, tx *sql.Tx) error {
		q := "SELECT COUNT(*) FROM " + table
		var args []interface{}
		if name != "" {
			q += " WHERE service_name = ?"
			args = []interface{}{name}
		}
		return tx.QueryRowContext(ctx, q, args...).Scan(&n)
	})
	return n, err
}
func (s *Store) GetServiceCount() (int64, error)           { return s.count("speedle_services", "") }
func (s *Store) GetPolicyCount(name string) (int64, error) { return s.count("speedle_policies", name) }
func (s *Store) GetRolePolicyCount(name string) (int64, error) {
	return s.count("speedle_role_policies", name)
}
func (s *Store) GetFunctionCount() (int64, error) { return s.count("speedle_functions", "") }
func (s *Store) GetPolicyAndRolePolicyCounts() (map[string]*pms.PolicyAndRolePolicyCount, error) {
	result := make(map[string]*pms.PolicyAndRolePolicyCount)
	err := s.transaction(context.Background(), false, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT s.name,(SELECT COUNT(*) FROM speedle_policies p WHERE p.service_name=s.name),(SELECT COUNT(*) FROM speedle_role_policies r WHERE r.service_name=s.name) FROM speedle_services s ORDER BY s.name`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			v := &pms.PolicyAndRolePolicyCount{}
			if err := rows.Scan(&name, &v.PolicyCount, &v.RolePolicyCount); err != nil {
				return err
			}
			result[name] = v
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) delete(table, where string, args []interface{}, single bool, parent string) error {
	return s.write(func(ctx context.Context, tx *sql.Tx) error {
		if parent != "" {
			if err := requireService(ctx, tx, parent); err != nil {
				return err
			}
		}
		q := "DELETE FROM " + table
		if where != "" {
			q += " WHERE " + where
		}
		result, err := tx.ExecContext(ctx, q, args...)
		if err != nil {
			return err
		}
		if single {
			n, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if n == 0 {
				return sql.ErrNoRows
			}
		}
		return nil
	})
}
func (s *Store) DeleteService(name string) error {
	return s.delete("speedle_services", "name = ?", []interface{}{name}, true, "")
}
func (s *Store) DeleteServices() error { return s.delete("speedle_services", "", nil, false, "") }
func (s *Store) DeletePolicy(name, id string) error {
	return s.delete("speedle_policies", "service_name = ? AND id = ?", []interface{}{name, id}, true, "")
}
func (s *Store) DeletePolicies(name string) error {
	return s.delete("speedle_policies", "service_name = ?", []interface{}{name}, false, name)
}
func (s *Store) DeleteRolePolicy(name, id string) error {
	return s.delete("speedle_role_policies", "service_name = ? AND id = ?", []interface{}{name, id}, true, "")
}
func (s *Store) DeleteRolePolicies(name string) error {
	return s.delete("speedle_role_policies", "service_name = ?", []interface{}{name}, false, name)
}
func (s *Store) DeleteFunction(name string) error {
	return s.delete("speedle_functions", "name = ?", []interface{}{name}, true, "")
}
func (s *Store) DeleteFunctions() error { return s.delete("speedle_functions", "", nil, false, "") }
