package flags

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
)

func TestGRPCEndpointConfigurationPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name, config, environment, cli, want string
	}{
		{"default", "", "", "", ""},
		{"json", "127.0.0.1:51001", "", "", "127.0.0.1:51001"},
		{"environment", "127.0.0.1:51001", "127.0.0.1:52001", "", "127.0.0.1:52001"},
		{"command line", "127.0.0.1:51001", "127.0.0.1:52001", "127.0.0.1:53001", "127.0.0.1:53001"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			originalArgs, originalFlags := os.Args, pflag.CommandLine
			t.Cleanup(func() { os.Args, pflag.CommandLine = originalArgs, originalFlags })
			pflag.CommandLine = pflag.NewFlagSet("grpc-config", pflag.ContinueOnError)
			file := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(file, []byte(`{"serverConfig":{"grpcEndpoint":"`+tc.config+`"}}`), 0600); err != nil {
				t.Fatal(err)
			}
			os.Args = []string{"grpc-config", "--config-file", file}
			if tc.cli != "" {
				os.Args = append(os.Args, "--grpc-endpoint", tc.cli)
			}
			t.Setenv("SPDL_GRPC_ENDPOINT", tc.environment)
			var parameters Parameters
			if err := parameters.ParseFlags("127.0.0.1:6733", func() {}, nil); err != nil {
				t.Fatal(err)
			}
			if parameters.GRPCEndpoint.Value != tc.want {
				t.Fatalf("got %q, want %q", parameters.GRPCEndpoint.Value, tc.want)
			}
		})
	}
}
