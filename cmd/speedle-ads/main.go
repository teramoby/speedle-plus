//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package main

import (
	"context"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/pkg/ads"
	"github.com/teramoby/speedle-plus/pkg/cmd/flags"
	"github.com/teramoby/speedle-plus/pkg/store"
	"os"
	"os/signal"
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := ads.Run(ctx, &params, conf); err != nil {
		log.Error(err)
		os.Exit(1)
	}
}
