//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package command

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/cmd/spctl/client"
	"github.com/teramoby/speedle-plus/pkg/pdl"
	"github.com/teramoby/speedle-plus/pkg/store"
	"github.com/teramoby/speedle-plus/pkg/store/file"
)

var (
	pdlFileName        string
	jsonFileName       string
	command            string
	serviceType        string
	funcURL            string
	funcResultCachable bool
	funcResultTTL      int64
)

var (
	createExample = `
	    # Create an empty service with name "service1" and default service type
		spctl create service service1

		# Create an empty service with name "service1" and type "k8s"
		spctl create service service1 --service-type=k8s

		# Create a service with policies using a service definition file in json format		
		spctl create service --json-file service.json

		# Create a service with policies and role policies using a file with policies in PDL format
		spctl create service service1 --service-type=k8s --pdl-file pdl.txt 
		sample pdl-file:
		--------------------------------------------------------
		role-policies:
		grant user User1 Role1 on res1
		grant group Group1 Role2 on res2
		policies:
		grant group Administrators GET,POST,DELETE expr:/service/* if request_time > '2017-09-04 12:00:00'
		grant user User1 GET /service/service1
		---------------------------------------------------------

		# Create a policy with name "p01" using pdl
		spctl create policy p01 --pdl-command "grant group Administrators list,watch,get expr:c1/default/core/pods/*" --service-name=service1

		# Create a poliy in service service1 using the data in policy.json.
		spctl create policy --json-file ./policy.json --service-name=service1

		# Create a role policy with name "rp01" using pdl
		spctl create rolepolicy rp01 --pdl-command "grant user User1 Role1 on res1" --service-name=service1

		# Create a role poliy in service service1 using the data in rolePolicy.json.
		spctl create rolepolicy --json-file ./rolePolicy.json --service-name=service1
		
		# Create a function "foo", funcUrl , cacheResult, cacheTTL 
		spctl create function foo --func-url=https://a.b.c:3456/funcs/foo --cachable=true --cache-ttl=3600

		# Create a function using function definition json file
		spctl create function --json-file=function.json`
)

func newCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create (service | policy | rolepolicy | function) (NAME | --json-file JSON_FILENAME) [--pdl-command COMMMAND] [--service-type=TYPE] [--pdl-file=PDL FILE NAME] [--service-name=NAME]",
		Short:   "Create a service | policy | role-policy",
		Example: createExample,
		Run:     createCommandFunc,
	}

	cmd.Flags().StringVarP(&serviceType, "service-type", "t", pms.TypeApplication, "service type, e.g. k8s")
	cmd.Flags().StringVarP(&serviceName, "service-name", "s", "", "service name")
	cmd.Flags().StringVarP(&command, "pdl-command", "c", "", "policy definition language command")
	cmd.Flags().StringVarP(&jsonFileName, "json-file", "f", "", "file that contains policy/role policy/service/function definition in json format")
	cmd.Flags().StringVarP(&pdlFileName, "pdl-file", "l", "", "file that contains policy/role policy definition in policy definition language format")
	cmd.Flags().StringVarP(&funcURL, "func-url", "", "", "URL for the function")
	cmd.Flags().BoolVarP(&funcResultCachable, "cachable", "", false, "whether the function result is cachable")
	cmd.Flags().Int64VarP(&funcResultTTL, "cache-ttl", "", 0, "How many seconds could the function result be kept in cache, 0 means the result could be kept in cache forever")
	return cmd
}

func parsePdlFile(pdlFileName string, serviceName, serviceType string) (*pms.Service, error) {
	f, err := os.Open(pdlFileName)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err = scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	service := pms.Service{Name: serviceName, Type: serviceType}
	isRolePolicy := false
	isPolicy := false
	for i, line := range lines {
		line = strings.Trim(line, " \t")
		if "policies:" == line {
			isPolicy = true
			isRolePolicy = false
		} else if "role-policies:" == line {
			isPolicy = false
			isRolePolicy = true
		} else {
			if isPolicy {
				var policy *pms.Policy
				name := pdlFileName + "_policy_" + strconv.Itoa(i+1)
				policy, _, err := pdl.ParsePolicy(line, name)
				if err == nil {
					service.Policies = append(service.Policies, policy)
				}
			}
			if isRolePolicy {
				var rolePolicy *pms.RolePolicy
				name := pdlFileName + "_role_policy_" + strconv.Itoa(i+1)
				rolePolicy, _, err := pdl.ParseRolePolicy(line, name)
				if err == nil {
					service.RolePolicies = append(service.RolePolicies, rolePolicy)
				}
			}
		}
	}

	return &service, err
}

func createCommandFunc(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		printHelpAndExit(cmd)
	}

	hc, err := httpClient()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	cli := &client.Client{
		PMSEndpoint: globalFlags.PMSEndpoint,
		HTTPClient:  hc,
	}
	var res string

	switch strings.ToLower(args[0]) {
	case "service":
		var buf []byte
		if len(args) == 1 {
			// --pdl-file or --json-file should be found
			if jsonFileName == "" && pdlFileName == "" {
				printHelpAndExit(cmd)
			}
			if jsonFileName != "" {
				buf, err = ioutil.ReadFile(jsonFileName)
			} else if pdlFileName != "" {
				fileStore, err := store.NewStore(file.StoreType, map[string]interface{}{
					file.FileLocationKey: pdlFileName,
				})
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}

				services, err := fileStore.ListAllServices()
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				buf, err = json.Marshal(services)
			}
		} else if len(args) == 2 {
			serviceName = args[1]
			if serviceName == "" || serviceType == "" {
				printHelpAndExit(cmd)
			}

			if pdlFileName == "" {
				service := pms.Service{Name: serviceName, Type: serviceType}
				buf, err = json.Marshal(service)
			} else {
				fileStore, err := store.NewStore(file.StoreType, map[string]interface{}{
					file.FileLocationKey: pdlFileName,
				})
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				if err == nil {
					service, err := fileStore.GetService(serviceName)
					if err != nil {
						fmt.Fprintln(os.Stderr, err)
						os.Exit(1)
					}
					buf, err = json.Marshal(service)
				}
			}

		}
		if err == nil {
			res, err = cli.Post([]string{"service"}, bytes.NewBuffer(buf), "")
		}

	case "policy", "rolepolicy":
		if serviceName == "" {
			printHelpAndExit(cmd)
		}
		var kind string
		if "policy" == strings.ToLower(args[0]) {
			kind = "policy"
		} else {
			kind = "role-policy"
		}
		if command != "" {
			var buf io.Reader
			var name string
			if len(args) == 2 {
				name = args[1]
			}
			if kind == "policy" {
				_, buf, err = pdl.ParsePolicy(command, name)

			} else {
				_, buf, err = pdl.ParseRolePolicy(command, name)
			}
			if err == nil {
				res, err = cli.Post([]string{"service", serviceName, kind}, buf, "")
			}
		} else {
			if len(args) != 1 || jsonFileName == "" {
				printHelpAndExit(cmd)
			}
			var buf []byte
			buf, err = ioutil.ReadFile(jsonFileName)
			if err == nil {
				res, err = cli.Post([]string{"service", serviceName, kind}, bytes.NewBuffer(buf), "")
			}
		}
	case "function":
		var buf []byte
		if len(args) == 1 {
			if jsonFileName == "" {
				printHelpAndExit(cmd)
			}
			buf, err = ioutil.ReadFile(jsonFileName)

		} else if len(args) == 2 {
			funcName := args[1]
			function := pms.Function{
				Name:           funcName,
				FuncURL:        funcURL,
				ResultCachable: funcResultCachable,
				ResultTTL:      funcResultTTL,
			}
			buf, err = json.Marshal(function)

		}
		if err == nil {
			res, err = cli.Post([]string{"function"}, bytes.NewBuffer(buf), "")
		}

	default:
		printHelpAndExit(cmd)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("%s created\n%s\n", args[0], res)
}
