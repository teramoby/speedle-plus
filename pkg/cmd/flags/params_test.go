//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package flags

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/teramoby/speedle-plus/pkg/errors"
	mysqlstore "github.com/teramoby/speedle-plus/pkg/store/mysql"
)

// exited is the panic value of the test exit function.
type exited int

// run calls f with exit replaced, and returns the exit code, or -1 when f
// returned normally.
func run(t *testing.T, f func()) (code int) {
	t.Helper()
	old := exit
	exit = func(code int) { panic(exited(code)) }
	defer func() {
		exit = old
		if r := recover(); r != nil {
			c, ok := r.(exited)
			if !ok {
				panic(r)
			}
			code = int(c)
		}
	}()
	f()
	return -1
}

// parse runs ParseFlags with the given arguments on a fresh flag set.
func parse(t *testing.T, args []string, storeParams map[string]string) (*Parameters, int) {
	t.Helper()
	oldArgs, oldFlags := os.Args, pflag.CommandLine
	os.Args = append([]string{"speedle"}, args...)
	pflag.CommandLine = pflag.NewFlagSet("speedle", pflag.ContinueOnError)
	pflag.CommandLine.SetOutput(io.Discard)
	t.Cleanup(func() { os.Args, pflag.CommandLine = oldArgs, oldFlags })
	params := &Parameters{}
	code := run(t, func() { params.ParseFlags("0.0.0.0:1", func() {}, storeParams) })
	return params, code
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFlagToEnv(t *testing.T) {
	if got := FlagToEnv("auditlog-max-size"); got != "SPDL_AUDITLOG_MAX_SIZE" {
		t.Fatalf("FlagToEnv = %q", got)
	}
}

func TestMySQLPropertyPrecedence(t *testing.T) {
	for _, source := range []string{"defaults", "file", "environment", "flag", "invalid"} {
		t.Run(source, func(t *testing.T) {
			oldArgs, oldFlags := os.Args, pflag.CommandLine
			defer func() { os.Args, pflag.CommandLine = oldArgs, oldFlags }()
			pflag.CommandLine = pflag.NewFlagSet("mysql-config", pflag.ContinueOnError)
			// Register the actual backend flags on the isolated parser, restoring
			// their values afterward so this does not contaminate other tests.
			mapping := (mysqlstore.MySQLStoreBuilder{}).GetStoreParams()
			for name := range mapping {
				flag := oldFlags.Lookup(name)
				value, changed := flag.Value.String(), flag.Changed
				defer func() { _ = flag.Value.Set(value); flag.Changed = changed }()
				_ = flag.Value.Set(flag.DefValue)
				flag.Changed = false
				pflag.CommandLine.AddFlag(flag)
			}
			os.Args = []string{"speedle"}
			wantDSN, wantSchema, wantPoll := "", "true", "1s"
			if source != "defaults" {
				path := writeFile(t, "mysql.json", `{"storeConfig":{"storeType":"mysql","storeProps":{"MySQLDSN":"from-file","MySQLInitSchema":false,"MySQLPollInterval":"2s"}}}`)
				os.Args = append(os.Args, "--config-file", path)
				wantDSN, wantSchema, wantPoll = "from-file", "false", "2s"
			}
			if source == "environment" || source == "flag" {
				t.Setenv("SPDL_MYSQLSTORE_DSN", "from-environment")
				t.Setenv("SPDL_MYSQLSTORE_POLL_INTERVAL", "3s")
				wantDSN, wantPoll = "from-environment", "3s"
			}
			if source == "flag" {
				os.Args = append(os.Args, "--mysqlstore-dsn=from-flag", "--mysqlstore-init-schema=true")
				wantDSN, wantSchema = "from-flag", "true"
			}
			if source == "invalid" {
				t.Setenv("SPDL_MYSQLSTORE_INIT_SCHEMA", "invalid")
				wantSchema = "invalid"
			}
			params := &Parameters{}
			if code := run(t, func() { params.ParseFlags("127.0.0.1:0", func() {}, mapping) }); code != -1 {
				t.Fatalf("exit %d", code)
			}
			configuration, err := params.Param2Config(mapping)
			if err != nil {
				t.Fatal(err)
			}
			properties := configuration.StoreConfig.StoreProps
			if properties[mysqlstore.DSNKey] != wantDSN || properties[mysqlstore.InitSchemaKey] != wantSchema || properties[mysqlstore.PollIntervalKey] != wantPoll {
				t.Fatalf("%s: %+v", source, properties)
			}
			if source == "invalid" {
				properties[mysqlstore.DSNKey] = "user:password@tcp(127.0.0.1:1)/db"
				if _, err := (mysqlstore.MySQLStoreBuilder{}).NewStore(properties); errors.Code(err) != errors.ConfigError {
					t.Fatalf("invalid flag reached connection or DDL: %v", err)
				}
			}
		})
	}
}

func TestParseFlagsDefaultsAndCommandLine(t *testing.T) {
	params, code := parse(t, []string{"--insecure=true", "--log-level=debug"}, nil)
	if code != -1 {
		t.Fatalf("exit %d", code)
	}
	if params.Endpoint.Value != "0.0.0.0:1" || params.Insecure.Value != "true" || params.LogConf.LogLevel.Value != "debug" {
		t.Fatalf("parameters %+v", params)
	}
	if params.StoreType.Value != DefaultStoreType || params.AuditLogConf.LogMaxSize.Value != DefaultAuditLogMaxSize {
		t.Fatalf("defaults not applied: %+v", params)
	}
}

func TestParseFlagsVersionExits(t *testing.T) {
	if _, code := parse(t, []string{"--version"}, nil); code != 0 {
		t.Fatalf("--version exit %d, want 0", code)
	}
}

func TestParseFlagsHelpListsEveryFlag(t *testing.T) {
	for _, arg := range []string{"--help", "-h"} {
		oldArgs, oldFlags := os.Args, pflag.CommandLine
		os.Args = []string{"speedle-test", arg}
		pflag.CommandLine = pflag.NewFlagSet("speedle", pflag.ContinueOnError)
		var out bytes.Buffer
		pflag.CommandLine.SetOutput(&out)
		params := &Parameters{}
		code := run(t, func() { params.ParseFlags("0.0.0.0:1", func() {}, nil) })
		os.Args, pflag.CommandLine = oldArgs, oldFlags
		if code != 0 {
			t.Fatalf("%s exit %d, want 0", arg, code)
		}
		help := out.String()
		for _, want := range []string{"Usage: speedle-test [flags]", "-h, --help", "--version", "--log-level", "--store-type", "SPDL_LOG_LEVEL"} {
			if !strings.Contains(help, want) {
				t.Errorf("%s output lacks %q:\n%s", arg, want, help)
			}
		}
	}
}

func TestParseFlagsFromEnvironment(t *testing.T) {
	t.Setenv("SPDL_ENDPOINT", "127.0.0.1:9")
	params, _ := parse(t, nil, nil)
	if params.Endpoint.Value != "127.0.0.1:9" {
		t.Fatalf("endpoint = %q", params.Endpoint.Value)
	}
}

const fullConfig = `{
 "storeConfig": {"storeType": "etcd", "storeProps": {"etcdEndpoint": "e:2379", "insecure": true, "keyPrefix": 3, "other": 1.5}},
 "enableWatch": true,
 "serverConfig": {"endpoint": "1.2.3.4:5", "insecure": "true", "enableAuthz": "true", "keyPath": "k", "certPath": "c",
   "clientCertPath": "cc", "forceClientCert": true},
 "logConfig": {"level": "warn", "formatter": "text", "setReportCaller": true,
   "rotationConfig": {"filename": "a.log", "maxsize": 1, "maxage": 2, "maxbackups": 3, "compress": true, "localtime": true}},
 "auditLogConfig": {"level": "info", "formatter": "json", "setReportCaller": true,
   "rotationConfig": {"filename": "b.log", "maxsize": 4, "maxage": 5, "maxbackups": 6, "compress": true, "localtime": true}},
 "asserterWebhookConfig": {"endpoint": "https://assert", "caCert": "ca", "clientCert": "cert", "clientKey": "key", "httpTimeout": 9}
}`

func TestParseFlagsFromConfigFile(t *testing.T) {
	config := writeFile(t, "config.json", fullConfig)
	storeParams := map[string]string{"etcd-endpoint": "etcdEndpoint", "etcd-insecure": "insecure", "etcd-prefix": "keyPrefix", "etcd-other": "other"}
	oldArgs, oldFlags := os.Args, pflag.CommandLine
	defer func() { os.Args, pflag.CommandLine = oldArgs, oldFlags }()
	os.Args = []string{"speedle", "--config-file", config}
	pflag.CommandLine = pflag.NewFlagSet("speedle", pflag.ContinueOnError)
	for flag := range storeParams {
		pflag.String(flag, "", "")
	}
	params := &Parameters{}
	if code := run(t, func() { params.ParseFlags("0.0.0.0:1", func() {}, storeParams) }); code != -1 {
		t.Fatalf("exit %d", code)
	}

	checks := []struct{ got, want string }{
		{params.Endpoint.Value, "1.2.3.4:5"}, {params.Insecure.Value, "true"}, {params.EnableAuthz.Value, "true"},
		{params.KeyPath.Value, "k"}, {params.CertPath.Value, "c"}, {params.ClientCertPath.Value, "cc"},
		{params.ForceClientCert.Value, "true"}, {params.StoreType.Value, "etcd"}, {params.StoreWatchEnabled.Value, "true"},
		{params.LogConf.LogLevel.Value, "warn"}, {params.LogConf.LogFormatter.Value, "text"},
		{params.LogConf.LogReportCaller.Value, "true"}, {params.LogConf.LogFileName.Value, "a.log"},
		{params.LogConf.LogMaxSize.Value, "1"}, {params.LogConf.LogMaxAge.Value, "2"}, {params.LogConf.LogMaxBackups.Value, "3"},
		{params.LogConf.LogCompress.Value, "true"}, {params.LogConf.LogLocalTime.Value, "true"},
		{params.AuditLogConf.LogLevel.Value, "info"}, {params.AuditLogConf.LogFormatter.Value, "json"},
		{params.AuditLogConf.LogReportCaller.Value, "true"}, {params.AuditLogConf.LogFileName.Value, "b.log"},
		{params.AuditLogConf.LogMaxSize.Value, "4"}, {params.AuditLogConf.LogMaxAge.Value, "5"},
		{params.AuditLogConf.LogMaxBackups.Value, "6"}, {params.AuditLogConf.LogCompress.Value, "true"},
		{params.AuditLogConf.LogLocalTime.Value, "true"},
		{params.AsserterConf.AsserterEndpoint.Value, "https://assert"}, {params.AsserterConf.AsserterCaPath.Value, "ca"},
		{params.AsserterConf.AsserterClientCertPath.Value, "cert"}, {params.AsserterConf.AsserterClientKeyPath.Value, "key"},
		{params.AsserterConf.AsserterClientTimeout.Value, "9"},
		{pflag.Lookup("etcd-endpoint").Value.String(), "e:2379"}, {pflag.Lookup("etcd-insecure").Value.String(), "true"},
		// JSON numbers are float64, which store parameters do not take.
		{pflag.Lookup("etcd-prefix").Value.String(), ""}, {pflag.Lookup("etcd-other").Value.String(), ""},
	}
	for i, c := range checks {
		if c.got != c.want {
			t.Errorf("check %d: got %q, want %q", i, c.got, c.want)
		}
	}

	conf, err := params.Param2Config(storeParams)
	if err != nil {
		t.Fatal(err)
	}
	if conf.StoreConfig.StoreType != "etcd" || conf.StoreConfig.StoreProps["etcdEndpoint"] != "e:2379" || !conf.EnableWatch {
		t.Errorf("store config %+v", conf.StoreConfig)
	}
	log := conf.LogConfig
	if log.Level != "warn" || log.Formatter != "text" || !log.SetReportCaller || log.RotationConfig.Filename != "a.log" ||
		log.RotationConfig.MaxSize != 1 || log.RotationConfig.MaxAge != 2 || log.RotationConfig.MaxBackups != 3 ||
		!log.RotationConfig.Compress || !log.RotationConfig.LocalTime {
		t.Errorf("log config %+v %+v", log, log.RotationConfig)
	}
	audit := conf.AuditLogConfig
	if audit.Level != "info" || audit.RotationConfig.Filename != "b.log" || audit.RotationConfig.MaxBackups != 6 ||
		!audit.RotationConfig.Compress || !audit.RotationConfig.LocalTime || !audit.SetReportCaller {
		t.Errorf("audit log config %+v %+v", audit, audit.RotationConfig)
	}
	asserter := conf.AsserterWebhookConfig
	if asserter.Endpoint != "https://assert" || asserter.CACert != "ca" || asserter.ClientCert != "cert" ||
		asserter.ClientKey != "key" || asserter.HTTPTimeout != 9 {
		t.Errorf("asserter config %+v", asserter)
	}
}

func TestLocalTimeIsNotCompress(t *testing.T) {
	params := &Parameters{}
	params.LogConf.LogFileName.Value = "a.log"
	params.LogConf.LogLocalTime.Value = "true"
	params.AuditLogConf.LogFileName.Value = "b.log"
	params.AuditLogConf.LogLocalTime.Value = "true"
	conf, _ := params.Param2Config(nil)
	for name, rotation := range map[string]bool{
		"log compress":   conf.LogConfig.RotationConfig.Compress,
		"audit compress": conf.AuditLogConfig.RotationConfig.Compress,
	} {
		if rotation {
			t.Errorf("%s was set by --*-localtime", name)
		}
	}
	if !conf.LogConfig.RotationConfig.LocalTime || !conf.AuditLogConfig.RotationConfig.LocalTime {
		t.Error("--*-localtime was ignored")
	}
}

func TestParam2ConfigWithoutOptionalSettings(t *testing.T) {
	params := &Parameters{}
	params.StoreType.Value = "file"
	params.LogConf.LogLevel.Value = "info"
	params.AsserterConf.AsserterEndpoint.Value = "https://a"
	params.AsserterConf.AsserterClientTimeout.Value = "x"
	conf, _ := params.Param2Config(nil)
	if conf.LogConfig.RotationConfig != nil || conf.AuditLogConfig != nil {
		t.Errorf("unexpected log config %+v / %+v", conf.LogConfig, conf.AuditLogConfig)
	}
	if conf.AsserterWebhookConfig.HTTPTimeout != 0 {
		t.Errorf("an invalid timeout was used: %+v", conf.AsserterWebhookConfig)
	}
}

func TestParseFlagsBadConfigFileExits(t *testing.T) {
	if _, code := parse(t, []string{"--config-file", writeFile(t, "bad.json", "{")}, nil); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	t.Setenv("SPDL_CONFIG_FILE", filepath.Join(t.TempDir(), "missing.json"))
	if _, code := parse(t, nil, nil); code != 1 {
		t.Fatalf("missing config from the environment: exit %d, want 1", code)
	}
}

func TestValidateFlags(t *testing.T) {
	param := func(insecure, authz, cert, key, force, client string) *Parameters {
		p := &Parameters{}
		p.Insecure.Value, p.EnableAuthz.Value, p.CertPath.Value, p.KeyPath.Value = insecure, authz, cert, key
		p.ForceClientCert.Value, p.ClientCertPath.Value = force, client
		return p
	}
	cases := []struct {
		name   string
		params *Parameters
		exits  bool
	}{
		{"insecure", param("true", "", "", "", "", ""), false},
		{"secure with certificates", param("false", "true", "c", "k", "true", "ca"), false},
		{"secure, client certificate optional", param("false", "", "c", "k", "false", ""), false},
		{"bad insecure", param("maybe", "", "", "", "", ""), true},
		{"bad enable-authz", param("true", "maybe", "", "", "", ""), true},
		{"secure without certificates", param("false", "", "", "", "", ""), true},
		{"bad force-client-cert", param("false", "", "c", "k", "maybe", ""), true},
		{"forced client certificate without CA", param("false", "", "c", "k", "true", ""), true},
	}
	oldFlags := pflag.CommandLine
	pflag.CommandLine = pflag.NewFlagSet("speedle", pflag.ContinueOnError)
	pflag.CommandLine.SetOutput(io.Discard)
	defer func() { pflag.CommandLine = oldFlags }()
	for _, c := range cases {
		code := run(t, c.params.ValidateFlags)
		if (code == 1) != c.exits {
			t.Errorf("%s: exit %d, want exit=%v", c.name, code, c.exits)
		}
	}
}

// writeCertificate writes a self-signed certificate and its key for
// 127.0.0.1 and returns their paths.
func writeCertificate(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "speedle-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPath = writeFile(t, "cert.pem", string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})))
	keyPath = writeFile(t, "key.pem", string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})))
	return certPath, keyPath
}

func TestServerTLSConfig(t *testing.T) {
	cert, key := writeCertificate(t)
	cases := []struct {
		name, clientCA, force string
		want                  tls.ClientAuthType
	}{
		{"no client CA", "", "", tls.NoClientCert},
		{"optional client certificate", cert, "false", tls.VerifyClientCertIfGiven},
		{"required client certificate", cert, "true", tls.RequireAndVerifyClientCert},
	}
	for _, c := range cases {
		params := &Parameters{}
		params.CertPath.Value, params.KeyPath.Value = cert, key
		params.ClientCertPath.Value, params.ForceClientCert.Value = c.clientCA, c.force
		config, err := params.serverTLSConfig()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if config.ClientAuth != c.want {
			t.Errorf("%s: client auth %v, want %v", c.name, config.ClientAuth, c.want)
		}
	}
}

func TestInvalidClientCAIsAnError(t *testing.T) {
	params := &Parameters{}
	params.ClientCertPath.Value = writeFile(t, "ca.pem", "not a certificate")
	config, err := params.serverTLSConfig()
	if err == nil || errors.Code(err) != errors.ConfigError {
		t.Fatalf("serverTLSConfig() = %v, %v; an unusable client CA must not disable client authentication", config, err)
	}
	params.ClientCertPath.Value = filepath.Join(t.TempDir(), "missing.pem")
	if _, err := params.serverTLSConfig(); errors.Code(err) != errors.ConfigError {
		t.Fatalf("missing client CA: %v", err)
	}
	if _, err := params.NewHTTPServer(http.NotFoundHandler()); err == nil {
		t.Fatal("NewHTTPServer must fail with an unusable client CA")
	}
	if _, err := params.GRPCServerOptions(); err == nil {
		t.Fatal("GRPCServerOptions must fail with an unusable client CA")
	}
}

func TestGRPCServerOptions(t *testing.T) {
	params := &Parameters{}
	params.Insecure.Value = "true"
	options, err := params.GRPCServerOptions()
	if err != nil || len(options) != 1 || !params.IsInsecure() {
		t.Fatalf("insecure options = %v, %v", options, err)
	}

	cert, key := writeCertificate(t)
	params.Insecure.Value = "false"
	params.CertPath.Value, params.KeyPath.Value = cert, key
	options, err = params.GRPCServerOptions()
	if err != nil || len(options) != 2 {
		t.Fatalf("TLS options = %v, %v", options, err)
	}
	params.KeyPath.Value = cert
	if _, err := params.GRPCServerOptions(); errors.Code(err) != errors.ConfigError {
		t.Fatalf("a bad key pair must fail: %v", err)
	}
}

func TestListenAndServe(t *testing.T) {
	cert, key := writeCertificate(t)
	pool := x509.NewCertPool()
	pem, _ := os.ReadFile(cert)
	pool.AppendCertsFromPEM(pem)
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}

	for _, insecure := range []string{"true", "false"} {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		address := listener.Addr().String()
		listener.Close()

		params := &Parameters{}
		params.Endpoint.Value, params.Insecure.Value = address, insecure
		params.CertPath.Value, params.KeyPath.Value = cert, key
		server, err := params.NewHTTPServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}))
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- params.ListenAndServe(server) }()

		scheme := "https"
		if insecure == "true" {
			scheme = "http"
		}
		var resp *http.Response
		for i := 0; i < 100; i++ {
			if resp, err = client.Get(scheme + "://" + address + "/"); err == nil {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if err != nil {
			t.Fatalf("insecure=%s: %v", insecure, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusTeapot {
			t.Errorf("insecure=%s: status %d", insecure, resp.StatusCode)
		}
		server.Close()
		if err := <-done; err != http.ErrServerClosed {
			t.Errorf("insecure=%s: ListenAndServe returned %v", insecure, err)
		}
	}
}
