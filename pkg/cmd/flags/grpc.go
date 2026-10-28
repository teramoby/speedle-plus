package flags

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// GRPCListenAddress keeps the default gRPC listener on the REST listener's host.
// An explicit address supports separate interfaces and ephemeral test ports.
func (k *Parameters) GRPCListenAddress(defaultPort string) (string, error) {
	if k.GRPCEndpoint.Value != "" {
		if _, _, err := net.SplitHostPort(k.GRPCEndpoint.Value); err != nil {
			return "", fmt.Errorf("invalid grpc-endpoint: %w", err)
		}
		return k.GRPCEndpoint.Value, nil
	}
	host, _, err := net.SplitHostPort(k.Endpoint.Value)
	if err != nil {
		return "", fmt.Errorf("invalid REST endpoint for gRPC listener: %w", err)
	}
	return net.JoinHostPort(host, defaultPort), nil
}

// NewGRPCServer applies the same transport policy as NewHTTPServer. Credentials
// are loaded before a listener opens; invalid secure configuration never falls
// back to plaintext.
func (k *Parameters) NewGRPCServer(options ...grpc.ServerOption) (*grpc.Server, error) {
	insecure, err := strconv.ParseBool(k.Insecure.Value)
	if err != nil {
		return nil, fmt.Errorf("invalid insecure value: %w", err)
	}
	if !insecure {
		config, err := k.serverTLSConfig()
		if err != nil {
			return nil, err
		}
		options = append(options, grpc.Creds(credentials.NewTLS(config)))
	}
	return grpc.NewServer(options...), nil
}

// serverTLSConfig is shared by REST and gRPC, including required client trust.
func (k *Parameters) serverTLSConfig() (*tls.Config, error) {
	certificate, err := tls.LoadX509KeyPair(k.CertPath.Value, k.KeyPath.Value)
	if err != nil {
		return nil, fmt.Errorf("load server certificate and key: %w", err)
	}
	config := &tls.Config{
		MinVersion:       tls.VersionTLS12,
		CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256},
		Certificates:     []tls.Certificate{certificate},
	}
	forceClientCert := false
	if k.ForceClientCert.Value != "" {
		forceClientCert, err = strconv.ParseBool(k.ForceClientCert.Value)
		if err != nil {
			return nil, fmt.Errorf("invalid force-client-cert value: %w", err)
		}
	}
	if k.ClientCertPath.Value == "" {
		if forceClientCert {
			return nil, fmt.Errorf("force-client-cert requires a client CA file")
		}
		return config, nil
	}
	ca, err := os.ReadFile(k.ClientCertPath.Value)
	if err != nil {
		return nil, fmt.Errorf("read client CA: %w", err)
	}
	config.ClientCAs = x509.NewCertPool()
	if !config.ClientCAs.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("client CA file contains no valid certificates")
	}
	config.ClientAuth = tls.VerifyClientCertIfGiven
	if forceClientCert {
		config.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return config, nil
}
