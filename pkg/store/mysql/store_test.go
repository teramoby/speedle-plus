package mysql

import (
	"context"
	"database/sql"
	stderrors "errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	driver "github.com/go-sql-driver/mysql"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

func mockStore(t *testing.T) (*Store, sqlmock.Sqlmock) {
	t.Helper()
	db, m, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{db: db, timeout: time.Second, pollInterval: 20 * time.Millisecond, secrets: []string{"secret-password"}}
	t.Cleanup(func() {
		s.StopWatch()
		// database/sql may roll back a canceled transaction in awaitDone.
		deadline := time.Now().Add(time.Second)
		for m.ExpectationsWereMet() != nil && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if err := m.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		// Do not append expectations here: canceled transactions can still be
		// disposing a driver connection, and sqlmock does not synchronize its
		// expectation slice against ExpectClose. Close itself is tested below.
		_ = s.Close()
	})
	return s, m
}
func expectLock(m sqlmock.Sqlmock) {
	m.ExpectBegin()
	m.ExpectQuery("SELECT revision.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(1))
}

func TestInvalidConfiguration(t *testing.T) {
	configs := []map[string]interface{}{
		nil, {DSNKey: 12}, {DSNKey: ""}, {DSNKey: "bad-secret-password"}, {DSNKey: "user:secret-password@tcp(localhost:3306)/"},
		{DSNKey: "u:p@tcp(localhost)/db", InitSchemaKey: "wrong"}, {DSNKey: "u:p@tcp(localhost)/db", InitSchemaKey: 12},
		{DSNKey: "u:p@tcp(localhost)/db", PollIntervalKey: "0s"}, {DSNKey: "u:p@tcp(localhost)/db", QueryTimeoutKey: false},
	}
	for _, cfg := range configs {
		_, err := (MySQLStoreBuilder{}).NewStore(cfg)
		if errors.Code(err) != errors.ConfigError || strings.Contains(err.Error(), "secret-password") {
			t.Fatalf("unsafe/invalid error: %v", err)
		}
	}
	// A syntactically valid configuration reaches Ping, then closes on failure.
	for _, value := range []interface{}{false, "false"} {
		_, err := (MySQLStoreBuilder{}).NewStore(map[string]interface{}{DSNKey: "u:secret-password@tcp(127.0.0.1:1)/db?timeout=20ms", InitSchemaKey: value, QueryTimeoutKey: "50ms"})
		if errors.Code(err) != errors.StoreError || strings.Contains(err.Error(), "secret-password") {
			t.Fatalf("connection error: %v", err)
		}
	}
	if len((MySQLStoreBuilder{}).GetStoreParams()) != 4 {
		t.Fatal("missing flags")
	}
}

func expectSchema(m sqlmock.Sqlmock, create bool, version, engines int) {
	m.ExpectPing()
	if create {
		for i := 0; i < 5; i++ {
			m.ExpectExec("CREATE TABLE IF NOT EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
		}
		m.ExpectExec("INSERT IGNORE").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	m.ExpectQuery("SELECT schema_version").WillReturnRows(sqlmock.NewRows([]string{"schema_version"}).AddRow(version))
	if version != 1 {
		return
	}
	for i := 0; i < 5; i++ {
		m.ExpectQuery("SELECT .* LIMIT 0").WillReturnRows(sqlmock.NewRows([]string{"column"}))
	}
	m.ExpectQuery("SELECT COUNT.*information_schema.tables").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(engines))
}
func TestSchemaInitialization(t *testing.T) {
	for _, create := range []bool{true, false} {
		t.Run(map[bool]string{true: "automatic", false: "no-ddl"}[create], func(t *testing.T) {
			s, m := mockStore(t)
			expectSchema(m, create, 1, 5)
			if err := s.initialize(create); err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Run("version", func(t *testing.T) {
		s, m := mockStore(t)
		expectSchema(m, false, 2, 5)
		if errors.Code(s.initialize(false)) != errors.ConfigError {
			t.Fatal("accepted future schema")
		}
	})
	t.Run("engine", func(t *testing.T) {
		s, m := mockStore(t)
		expectSchema(m, false, 1, 4)
		if errors.Code(s.initialize(false)) != errors.ConfigError {
			t.Fatal("accepted nontransactional table")
		}
	})
	t.Run("ddl-failure", func(t *testing.T) {
		s, m := mockStore(t)
		m.ExpectPing()
		m.ExpectExec("CREATE TABLE").WillReturnError(stderrors.New("DDL denied"))
		if s.initialize(true) == nil {
			t.Fatal("ignored DDL failure")
		}
	})
	t.Run("missing-schema", func(t *testing.T) {
		s, m := mockStore(t)
		m.ExpectPing()
		m.ExpectQuery("SELECT schema_version").WillReturnError(sql.ErrNoRows)
		if s.initialize(false) == nil {
			t.Fatal("accepted missing schema")
		}
	})
}

func TestTransactionFailureBoundaries(t *testing.T) {
	fault := stderrors.New("fault")
	for _, stage := range []string{"begin", "lock", "operation", "revision", "affected", "exhausted", "commit"} {
		t.Run(stage, func(t *testing.T) {
			s, m := mockStore(t)
			if stage == "begin" {
				m.ExpectBegin().WillReturnError(fault)
			} else {
				m.ExpectBegin()
				q := m.ExpectQuery("SELECT revision.*FOR UPDATE")
				if stage == "lock" {
					q.WillReturnError(fault)
					m.ExpectRollback()
				} else {
					q.WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(1))
					if stage == "operation" {
						m.ExpectRollback()
					} else {
						e := m.ExpectExec("UPDATE speedle_store_meta")
						switch stage {
						case "revision":
							e.WillReturnError(fault)
						case "affected":
							e.WillReturnResult(sqlmock.NewErrorResult(fault))
						case "exhausted":
							e.WillReturnResult(sqlmock.NewResult(0, 0))
						default:
							e.WillReturnResult(sqlmock.NewResult(0, 1))
						}
						if stage == "commit" {
							m.ExpectCommit().WillReturnError(fault)
						} else {
							m.ExpectRollback()
						}
					}
				}
			}
			err := s.write(func(context.Context, *sql.Tx) error {
				if stage == "operation" {
					return fault
				}
				return nil
			})
			if errors.Code(err) != errors.StoreError {
				t.Fatalf("%s: %v", stage, err)
			}
		})
	}
}

func TestFailedBulkWriteLeavesInputUntouched(t *testing.T) {
	s, m := mockStore(t)
	input := &pms.Service{Name: "app", Policies: []*pms.Policy{{ID: "original", Name: "one"}}, RolePolicies: []*pms.RolePolicy{{ID: "original-role"}}}
	expectLock(m)
	m.ExpectExec("INSERT INTO speedle_services").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec("INSERT INTO speedle_policies").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec("INSERT INTO speedle_role_policies").WillReturnError(stderrors.New("disk full"))
	m.ExpectRollback()
	if s.CreateService(input) == nil {
		t.Fatal("expected rollback")
	}
	if input.Policies[0].ID != "original" || input.RolePolicies[0].ID != "original-role" {
		t.Fatal("failed transaction changed caller data")
	}

	expectLock(m)
	m.ExpectExec("DELETE FROM speedle_services").WillReturnResult(sqlmock.NewResult(0, 2))
	m.ExpectExec("DELETE FROM speedle_functions").WillReturnError(stderrors.New("disk full"))
	m.ExpectRollback()
	if s.WritePolicyStore(&pms.PolicyStore{}) == nil {
		t.Fatal("expected atomic replacement failure")
	}
}

func TestErrorCodesAndRedaction(t *testing.T) {
	s, m := mockStore(t)
	for err, want := range map[error]errors.ErrorCode{sql.ErrNoRows: errors.EntityNotFound, &driver.MySQLError{Number: 1062}: errors.EntityAlreadyExists, &driver.MySQLError{Number: 1452}: errors.EntityNotFound, &driver.MySQLError{Number: 1406}: errors.InvalidRequest} {
		if errors.Code(s.translate(err)) != want {
			t.Fatalf("%v", s.translate(err))
		}
	}
	if strings.Contains(s.translate(stderrors.New("secret-password")).Error(), "secret-password") {
		t.Fatal("credential leak")
	}
	expectLock(m)
	m.ExpectExec("DELETE FROM speedle_services").WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectRollback()
	if errors.Code(s.DeleteService("missing")) != errors.EntityNotFound {
		t.Fatal("missing delete must fail")
	}
}

func expectSnapshot(m sqlmock.Sqlmock, revision int64, empty bool) {
	m.ExpectBegin()
	m.ExpectQuery("SELECT revision").WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(revision))
	if empty {
		m.ExpectQuery("SELECT .* FROM speedle_services ORDER BY name").WillReturnRows(sqlmock.NewRows(strings.Split(serviceColumns, ",")))
		m.ExpectQuery("SELECT .* FROM speedle_functions ORDER BY name").WillReturnRows(sqlmock.NewRows(strings.Split(functionColumns, ",")))
	}
	m.ExpectCommit()
}
func TestWatchRetriesWithoutLosingRevision(t *testing.T) {
	s, m := mockStore(t)
	expectSnapshot(m, 1, true)
	m.ExpectBegin()
	m.ExpectQuery("SELECT revision").WillReturnError(stderrors.New("disconnected"))
	m.ExpectRollback()
	expectSnapshot(m, 2, true)
	events, err := s.Watch()
	if err != nil {
		t.Fatal(err)
	}
	again, _ := s.Watch()
	if events != again {
		t.Fatal("duplicate watcher")
	}
	for _, expected := range []struct {
		kind     pms.EventType
		revision int64
	}{{pms.FULL_RELOAD, 1}, {pms.STORE_UNAVAILABLE, 0}, {pms.FULL_RELOAD, 2}} {
		select {
		case e := <-events:
			if e.ID != expected.revision || e.Type != expected.kind {
				t.Fatalf("event %+v", e)
			}
			if e.Type == pms.FULL_RELOAD && e.Content.(*pms.PolicyStore) == nil {
				t.Fatal("missing recovery snapshot")
			}
		case <-time.After(time.Second):
			t.Fatal("watch did not recover")
		}
	}
	s.StopWatch()
	s.StopWatch()
	if _, open := <-events; open {
		t.Fatal("watch channel still open")
	}
}
func TestStopWatchCancelsBlockedDeliveryAndQuery(t *testing.T) {
	for _, blocked := range []string{"send", "query"} {
		t.Run(blocked, func(t *testing.T) {
			s, m := mockStore(t)
			if blocked == "send" {
				expectSnapshot(m, 1, true)
				expectSnapshot(m, 2, true)
			} else {
				m.ExpectBegin()
				m.ExpectQuery("SELECT revision").WillDelayFor(time.Second).WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(1))
				m.ExpectRollback()
			}
			_, err := s.Watch()
			if err != nil {
				t.Fatal(err)
			}
			time.Sleep(60 * time.Millisecond)
			start := time.Now()
			s.StopWatch()
			if time.Since(start) > 500*time.Millisecond {
				t.Fatal("StopWatch blocked")
			}
		})
	}
}
func TestClosedStore(t *testing.T) {
	s, m := mockStore(t)
	m.ExpectClose()
	// Close twice: closing an already closed store must also succeed.
	first, second := s.Close(), s.Close()
	if first != nil || second != nil {
		t.Fatal("close failed")
	}
	if _, err := s.Watch(); errors.Code(err) != errors.StoreError {
		t.Fatal("watch after close")
	}
}

func TestNameFiltersAndKeys(t *testing.T) {
	for filter, want := range map[string]bool{"": true, "name": true, "id eq 1": true, "name xx x": true, "name eq alpha": true, "name eq beta": false, "name co ph": true, "name sw al": true, "name pr": true, "name gt beta": false, "name ge alpha": true, "name lt beta": true, "name le alpha": true, "names eq x": true} {
		if nameMatches("alpha", filter) != want {
			t.Errorf("filter %q", filter)
		}
	}
	for _, key := range []string{"", strings.Repeat("界", 256), string([]byte{0xff})} {
		if validKey(key) == nil {
			t.Fatal("invalid key accepted")
		}
	}
	if validKey(strings.Repeat("界", 255)) != nil {
		t.Fatal("valid Unicode key rejected")
	}
	var target []string
	if jsonInto(&target).Scan(`[]`) != nil || jsonInto(&target).Scan([]byte(`null`)) != nil || jsonInto(&target).Scan(1) == nil || jsonInto(&target).Scan(`broken`) == nil {
		t.Fatal("JSON scanning")
	}
}
