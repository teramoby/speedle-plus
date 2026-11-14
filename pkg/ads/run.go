//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

// Package ads provides the reusable authorization decision server startup.
package ads

import (
	"context"
	"io"
	"net"
	"net/http"
	"time"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/assertion"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/cmd/flags"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/logging"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsgrpc"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsgrpc/pb"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsrest"

	log "github.com/sirupsen/logrus"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// Run starts the decision servers until a server fails or ctx is cancelled.
// It owns the evaluator and its store/watch lifecycle. Register custom stores
// before calling Run. The caller owns signal handling and extra flag parsing.
func Run(ctx context.Context, params *flags.Parameters, conf *cfg.Config, options ...Option) error {
	settings := settings{shutdownTimeout: 5 * time.Second}
	for _, option := range options {
		option(&settings)
	}

	if settings.httpListener != nil {
		defer settings.httpListener.Close()
	}
	if settings.grpcListener != nil {
		defer settings.grpcListener.Close()
	}
	if settings.shutdownTimeout <= 0 {
		return errors.New(errors.ConfigError, "shutdown timeout must be positive")
	}

	// Initialize the logging
	if conf.LogConfig != nil {
		err := logging.InitLog(conf.LogConfig)
		if err != nil {
			log.Errorf("Authz_check failed to initialize the log module, err: %v.", err)
		}
	} else {
		log.Error("No any log configurations for authorization service.")
	}

	// Initialize the Audit logging
	if conf.AuditLogConfig != nil {
		err := logging.InitAuditLog(conf.AuditLogConfig)
		if err != nil {
			log.Errorf("Authz_check failed to initialize the audit log module, err: %v.", err)
		}
	} else {
		log.Error("No any audit log configurations for authorization service.\n")
	}

	evaluator, err := newEvaluator(conf)
	if err != nil {
		return err
	}
	if closer, ok := evaluator.(io.Closer); ok {
		defer closer.Close()
	}

	httpServer, err := newHTTPServer(params, evaluator, settings)
	if err != nil {
		return err
	}

	var grpcServer *grpc.Server
	grpcEndpoint := params.GRPCEndpoint.Value
	if grpcEndpoint != "" || settings.grpcListener != nil {
		grpcServer, err = newGRPCServer(params, evaluator, settings)
		if err != nil {
			return err
		}
	} else {
		log.Info("The gRPC server is off; set --grpc-endpoint to start it.")
	}

	errChan := make(chan error, 2)
	if grpcServer != nil {
		go func() {
			log.Infof("Starting the gRPC server for authorization service on %s...", grpcEndpoint)
			if settings.grpcListener != nil {
				errChan <- grpcServer.Serve(settings.grpcListener)
			} else {
				errChan <- listenGRPCServer(grpcServer, grpcEndpoint)
			}
		}()
	}

	go func() {
		log.Info("Starting the REST server for authorization service...")
		if settings.httpListener != nil {
			if params.IsInsecure() {
				errChan <- httpServer.Serve(settings.httpListener)
			} else {
				errChan <- httpServer.ServeTLS(settings.httpListener, params.CertPath.Value, params.KeyPath.Value)
			}
		} else {
			errChan <- params.ListenAndServe(httpServer)
		}
	}()

	// errChan is never closed: the other server may still report an error.
	select {
	case err = <-errChan:
	case <-ctx.Done():
		log.Info("Interrupt signal")
	}

	log.Info("Stopping servers...")
	// Stop gRPC independently of active HTTP requests.
	if grpcServer != nil {
		log.Info("Stopping GRPC Server.")
		grpcServer.Stop()
	}
	if httpServer != nil {
		log.Info("Stopping HTTP Server.")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), settings.shutdownTimeout)
		defer cancel()
		if shutdownErr := httpServer.Shutdown(shutdownCtx); shutdownErr != nil {
			_ = httpServer.Close()
		}
	}

	return err
}

func newEvaluator(conf *cfg.Config) (eval.InternalEvaluator, error) {
	evaluator, err := eval.NewFromConfig(conf)
	if err != nil {
		return nil, err
	}

	log.Info("Loading asserters.")
	as, errLoadAsserter := assertion.NewAsserter(conf.AsserterWebhookConfig, nil)
	if errLoadAsserter != nil {
		log.Warningf("load asserter error: %v", errLoadAsserter)
	} else {
		f := func(ctx *adsapi.RequestContext) error {
			if ctx.Subject != nil &&
				len(ctx.Subject.TokenType) != 0 &&
				len(ctx.Subject.Token) != 0 {
				tokenType := ctx.Subject.TokenType
				token := ctx.Subject.Token
				log.Debugf("Asserting token with token type %s.", tokenType)
				s, err := as.AssertToken(token, tokenType, "", nil)
				if err == nil {
					ctx.Subject.Principals = append(ctx.Subject.Principals, s.Principals...)
				} else {
					log.Errorf("Failed to assert due to error %v.", err)
					return err
				}
			}
			return nil
		}
		evaluator.SetAsserterFunc(f)
	}

	return evaluator, nil
}

func newGRPCServer(params *flags.Parameters, evaluator eval.InternalEvaluator, settings settings) (*grpc.Server, error) {

	var serviceOptions []adsgrpc.Option
	if settings.gateSet {
		serviceOptions = append(serviceOptions, adsgrpc.WithGate(settings.gate))
	}
	serviceImpl, err := adsgrpc.NewGRPCService(evaluator, serviceOptions...)
	if err != nil {
		return nil, err
	}

	options, err := params.GRPCServerOptions()
	if err != nil {
		return nil, err
	}
	if params.IsInsecure() {
		log.Warningf("The gRPC server on %s is plaintext because insecure is true.", params.GRPCEndpoint.Value)
	}
	options = append(options, settings.grpcOptions...)
	server := grpc.NewServer(options...)
	pb.RegisterEvaluatorServer(server, serviceImpl)
	// Register reflection service on gRPC server.
	reflection.Register(server)
	return server, nil
}

func listenGRPCServer(server *grpc.Server, endpoint string) error {
	lis, err := net.Listen("tcp", endpoint)
	if err != nil {
		return errors.Wrapf(err, errors.ServerError, "failed to listen on endpoint %s", endpoint)
	}
	if err := server.Serve(lis); err != nil {
		return errors.Wrapf(err, errors.ServerError, "failed to serve for endpoint %s", endpoint)
	}
	return nil
}

func newHTTPServer(params *flags.Parameters, evaluator eval.InternalEvaluator, settings settings) (*http.Server, error) {
	var serviceOptions []adsrest.Option
	if settings.gateSet {
		serviceOptions = append(serviceOptions, adsrest.WithGate(settings.gate))
	}
	routers, err := adsrest.NewRouter(evaluator, serviceOptions...)
	if err != nil {
		return nil, err
	}
	server, err := params.NewHTTPServer(routers)
	if err == nil && settings.httpServer != nil {
		settings.httpServer(server)
	}
	return server, err
}
