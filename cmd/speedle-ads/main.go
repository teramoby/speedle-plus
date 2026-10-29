//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"

	adsapi "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/assertion"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/cmd/flags"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/logging"
	"github.com/teramoby/speedle-plus/pkg/store"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsgrpc"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsgrpc/pb"
	"github.com/teramoby/speedle-plus/pkg/svcs/adsrest"

	log "github.com/sirupsen/logrus"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var gitCommit string
var productVersion string
var goVersion string

func printVersionInfo() {
	fmt.Printf("speedle-ads:\n")
	fmt.Printf(" Version:       %s\n", productVersion)
	fmt.Printf(" Go Version:    %s\n", goVersion)
	fmt.Printf(" Git commit:    %s\n", gitCommit)
}

func main() {

	storeParamsMap := store.GetAllStoreParams()

	var params flags.Parameters
	params.ParseFlags(flags.DefaultAuthzCheckEndPoint, printVersionInfo, storeParamsMap)
	params.ValidateFlags()

	conf, _ := params.Param2Config(storeParamsMap)

	intChan := make(chan os.Signal, 1)
	signal.Notify(intChan, os.Interrupt)
	if err := run(&params, conf, intChan); err != nil {
		log.Error(err)
		os.Exit(1)
	}
}

// run starts the servers and serves until one of them fails or stop
// receives a signal.
func run(params *flags.Parameters, conf *cfg.Config, stop <-chan os.Signal) error {
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

	httpServer, err := newHTTPServer(params, evaluator)
	if err != nil {
		return err
	}

	var grpcServer *grpc.Server
	grpcEndpoint := params.GRPCEndpoint.Value
	if grpcEndpoint != "" {
		grpcServer, err = newGRPCServer(params, evaluator)
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
			errChan <- listenGRPCServer(grpcServer, grpcEndpoint)
		}()
	}

	go func() {
		log.Info("Starting the REST server for authorization service...")
		errChan <- params.ListenAndServe(httpServer)
	}()

	// errChan is never closed: the other server may still report an error.
	select {
	case err = <-errChan:
	case <-stop:
		log.Info("Interrupt signal")
	}

	log.Info("Stopping servers...")
	// Stop all services
	if httpServer != nil {
		log.Info("Stopping HTTP Server.")
		httpServer.Shutdown(context.Background())
	}
	if grpcServer != nil {
		log.Info("Stopping GRPC Server.")
		grpcServer.Stop()
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

func newGRPCServer(params *flags.Parameters, evaluator eval.InternalEvaluator) (*grpc.Server, error) {

	serviceImpl, err := adsgrpc.NewGRPCService(evaluator)
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

func newHTTPServer(params *flags.Parameters, evaluator eval.InternalEvaluator) (*http.Server, error) {
	routers, err := adsrest.NewRouter(evaluator)
	if err != nil {
		return nil, err
	}
	return params.NewHTTPServer(routers)
}
