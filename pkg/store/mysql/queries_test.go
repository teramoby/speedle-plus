package mysql

import (
	"database/sql"
	stderrors "errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

func sampleService() *sqlmock.Rows {
	return sqlmock.NewRows(strings.Split(serviceColumns, ",")).AddRow("app", "application", `{"team":"a"}`)
}
func samplePolicy() *sqlmock.Rows {
	return sqlmock.NewRows(strings.Split(policyColumns, ",")).AddRow("p", "policy", "grant", `[{"resource":"r","actions":["get"]}]`, `[["user:alice"]]`, "", `null`)
}
func sampleRole() *sqlmock.Rows {
	return sqlmock.NewRows(strings.Split(roleColumns, ",")).AddRow("rp", "role", "grant", `["admin"]`, `["user:alice"]`, `null`, `null`, "", `null`)
}
func sampleFunction() *sqlmock.Rows {
	return sqlmock.NewRows(strings.Split(functionColumns, ",")).AddRow("f", "desc", "https://f", "", "", true, 5, `null`)
}

func TestSnapshotIsReadInOneTransaction(t *testing.T) {
	s, m := mockStore(t)
	m.ExpectBegin()
	m.ExpectQuery("SELECT revision").WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(7))
	m.ExpectQuery("SELECT .* FROM speedle_services ORDER BY name").WillReturnRows(sampleService())
	m.ExpectQuery("SELECT .* FROM speedle_policies").WithArgs("app").WillReturnRows(samplePolicy())
	m.ExpectQuery("SELECT .* FROM speedle_role_policies").WithArgs("app").WillReturnRows(sampleRole())
	m.ExpectQuery("SELECT .* FROM speedle_functions").WillReturnRows(sampleFunction())
	m.ExpectCommit()
	ps, err := s.ReadPolicyStore()
	if err != nil || ps.Services[0].Policies[0].Permissions[0].Resource != "r" || ps.Services[0].RolePolicies[0].Roles[0] != "admin" || !ps.Functions[0].ResultCachable {
		t.Fatalf("snapshot %+v %v", ps, err)
	}
	expectSnapshot(m, 7, false)
	seen := int64(7)
	if ps, _, err := s.snapshot(t.Context(), &seen); err != nil || ps != nil {
		t.Fatalf("unchanged snapshot %+v %v", ps, err)
	}
}

func TestReadsAndCounts(t *testing.T) {
	s, m := mockStore(t)
	m.ExpectBegin()
	m.ExpectQuery("SELECT .* FROM speedle_policies").WithArgs("app", "p").WillReturnRows(samplePolicy())
	m.ExpectCommit()
	if p, err := s.GetPolicy("app", "p"); err != nil || p.ID != "p" {
		t.Fatalf("policy %v %v", p, err)
	}
	m.ExpectBegin()
	m.ExpectQuery("SELECT .* FROM speedle_role_policies").WithArgs("app", "rp").WillReturnRows(sampleRole())
	m.ExpectCommit()
	if p, err := s.GetRolePolicy("app", "rp"); err != nil || p.ID != "rp" {
		t.Fatalf("role %v %v", p, err)
	}
	m.ExpectBegin()
	m.ExpectQuery("SELECT .* FROM speedle_functions").WithArgs("f").WillReturnRows(sampleFunction())
	m.ExpectCommit()
	if f, err := s.GetFunction("f"); err != nil || f.ResultTTL != 5 {
		t.Fatalf("function %v %v", f, err)
	}
	m.ExpectBegin()
	m.ExpectQuery("SELECT name FROM speedle_services ORDER BY name").WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("app"))
	m.ExpectCommit()
	if names, err := s.GetServiceNames(); err != nil || len(names) != 1 {
		t.Fatal(names, err)
	}
	for _, count := range []func() (int64, error){s.GetServiceCount, s.GetFunctionCount, func() (int64, error) { return s.GetPolicyCount("app") }, func() (int64, error) { return s.GetRolePolicyCount("") }} {
		m.ExpectBegin()
		m.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
		m.ExpectCommit()
		if n, err := count(); err != nil || n != 3 {
			t.Fatal(n, err)
		}
	}
	m.ExpectBegin()
	m.ExpectQuery("SELECT s.name").WillReturnRows(sqlmock.NewRows([]string{"name", "policies", "roles"}).AddRow("app", 2, 3))
	m.ExpectCommit()
	if counts, err := s.GetPolicyAndRolePolicyCounts(); err != nil || counts["app"].RolePolicyCount != 3 {
		t.Fatal(counts, err)
	}
}

func TestMissingOrCorruptReadsNeverReturnPartialData(t *testing.T) {
	s, m := mockStore(t)
	m.ExpectBegin()
	m.ExpectQuery("SELECT .* FROM speedle_functions").WillReturnRows(sqlmock.NewRows(strings.Split(functionColumns, ",")))
	m.ExpectRollback()
	if f, err := s.GetFunction("missing"); f != nil || errors.Code(err) != errors.EntityNotFound {
		t.Fatal(f, err)
	}
	m.ExpectBegin()
	m.ExpectQuery("SELECT .* FROM speedle_policies").WillReturnRows(sqlmock.NewRows(strings.Split(policyColumns, ",")).AddRow("p", "p", "grant", `broken`, `null`, "", `null`))
	m.ExpectRollback()
	if p, err := s.GetPolicy("app", "p"); p != nil || err == nil {
		t.Fatal("corrupt JSON accepted")
	}
	m.ExpectBegin()
	m.ExpectQuery("SELECT .* FROM speedle_functions").WillReturnRows(sampleFunction().RowError(0, stderrors.New("connection lost")))
	m.ExpectRollback()
	if _, err := s.ListAllFunctions(""); errors.Code(err) != errors.StoreError {
		t.Fatal(err)
	}
	m.ExpectBegin()
	m.ExpectQuery("SELECT .* FROM speedle_services WHERE name").WillReturnError(sql.ErrNoRows)
	m.ExpectRollback()
	if _, err := s.GetService("missing"); errors.Code(err) != errors.EntityNotFound {
		t.Fatal(err)
	}
}
