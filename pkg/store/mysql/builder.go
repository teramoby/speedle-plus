// Licensed under the Universal Permissive License (UPL) Version 1.0.
package mysql

import (
	"context"
	"database/sql"
	_ "embed"
	"strconv"
	"strings"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/spf13/pflag"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/store"
)

const (
	StoreType       = "mysql"
	DSNKey          = "MySQLDSN"
	InitSchemaKey   = "MySQLInitSchema"
	PollIntervalKey = "MySQLPollInterval"
	QueryTimeoutKey = "MySQLQueryTimeout"
)

// SchemaSQL is also shipped as schema.sql for deployments without DDL privileges.
//
//go:embed schema.sql
var SchemaSQL string

type MySQLStoreBuilder struct{}

func (MySQLStoreBuilder) GetStoreParams() map[string]string {
	return map[string]string{"mysqlstore-dsn": DSNKey, "mysqlstore-init-schema": InitSchemaKey,
		"mysqlstore-poll-interval": PollIntervalKey, "mysqlstore-query-timeout": QueryTimeoutKey}
}

func init() {
	pflag.String("mysqlstore-dsn", "", "Store config: MySQL DSN (including an existing database).")
	// Keep the raw value until NewStore validates it. The shared flags/env
	// merger ignores Value.Set errors, so a Bool flag would silently replace
	// an invalid environment/config value with the default (true).
	pflag.String("mysqlstore-init-schema", "true", "Store config: create MySQL tables if absent (true/false).")
	pflag.String("mysqlstore-poll-interval", "1s", "Store config: MySQL watch polling interval.")
	pflag.String("mysqlstore-query-timeout", "10s", "Store config: MySQL operation timeout.")
	store.Register(StoreType, MySQLStoreBuilder{})
}

func duration(config map[string]interface{}, key string, fallback time.Duration) (time.Duration, error) {
	v, exists := config[key]
	if !exists {
		return fallback, nil
	}
	text, ok := v.(string)
	if ok {
		d, err := time.ParseDuration(text)
		if err == nil && d > 0 {
			return d, nil
		}
	}
	return 0, errors.Errorf(errors.ConfigError, "%s must be a positive duration", key)
}

func (MySQLStoreBuilder) NewStore(config map[string]interface{}) (pms.PolicyStoreManager, error) {
	dsn, ok := config[DSNKey].(string)
	if !ok || dsn == "" {
		return nil, errors.New(errors.ConfigError, "MySQLDSN is required")
	}
	cfg, err := driver.ParseDSN(dsn)
	// Parse errors can contain DSN fragments. Never return the original error.
	if err != nil || cfg.DBName == "" {
		return nil, errors.New(errors.ConfigError, "invalid MySQLDSN; an existing database is required")
	}
	poll, err := duration(config, PollIntervalKey, time.Second)
	if err != nil {
		return nil, err
	}
	timeout, err := duration(config, QueryTimeoutKey, 10*time.Second)
	if err != nil {
		return nil, err
	}
	initialize := true
	if v, exists := config[InitSchemaKey]; exists {
		switch v := v.(type) {
		case bool:
			initialize = v
		case string:
			initialize, err = strconv.ParseBool(v)
		default:
			err = errors.New(errors.ConfigError, "invalid boolean")
		}
		if err != nil {
			return nil, errors.New(errors.ConfigError, "MySQLInitSchema must be a boolean")
		}
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = timeout
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = timeout
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = timeout
	}
	connector, err := driver.NewConnector(cfg)
	if err != nil {
		return nil, errors.New(errors.ConfigError, "invalid MySQL connection configuration")
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(3 * time.Minute)
	s := &Store{db: db, timeout: timeout, pollInterval: poll, secrets: []string{dsn, cfg.Passwd}}
	if err := s.initialize(initialize); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) initialize(create bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	if err := s.db.PingContext(ctx); err != nil {
		return s.translate(err)
	}
	if create {
		// schema.sql contains only fixed statements; never enable multiStatements.
		var statements []string
		for _, line := range strings.Split(SchemaSQL, "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "--") {
				statements = append(statements, line)
			}
		}
		for _, stmt := range strings.Split(strings.Join(statements, "\n"), ";") {
			if strings.TrimSpace(stmt) != "" {
				if _, err := s.db.ExecContext(ctx, stmt); err != nil {
					return s.translate(err)
				}
			}
		}
	}
	var version int
	if err := s.db.QueryRowContext(ctx, "SELECT schema_version FROM speedle_store_meta WHERE id = 1").Scan(&version); err != nil {
		return s.translate(err)
	}
	if version != 1 {
		return errors.New(errors.ConfigError, "unsupported MySQL schema version (expected 1)")
	}
	for _, table := range []struct{ name, columns string }{
		{"speedle_services", serviceColumns}, {"speedle_policies", policyColumns},
		{"speedle_role_policies", roleColumns}, {"speedle_functions", functionColumns},
		{"speedle_store_meta", "id,schema_version,revision"},
	} {
		rows, err := s.db.QueryContext(ctx, "SELECT "+table.columns+" FROM "+table.name+" LIMIT 0")
		if err != nil {
			return s.translate(err)
		}
		_ = rows.Close()
	}
	var tables int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name IN ('speedle_store_meta','speedle_services','speedle_policies','speedle_role_policies','speedle_functions') AND engine = 'InnoDB'`).Scan(&tables); err != nil {
		return s.translate(err)
	}
	if tables != 5 {
		return errors.New(errors.ConfigError, "MySQL policy tables must use InnoDB")
	}
	return nil
}
