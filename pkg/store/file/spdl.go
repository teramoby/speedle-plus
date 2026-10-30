//Copyright (c) 2019, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package file

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/pdl"
	"github.com/teramoby/speedle-plus/pkg/suid"
)

type phase int

const (
	phaseRoot phase = iota
	phaseService
	phasePolicy
	phaseRolepolicy
	phaseUnknown
)

func (t phase) String() string {
	name := []string{"phaseRoot", "phaseService", "phasePolicy", "phaseRolepolicy", "phaseUnknown"}
	i := int(t)
	switch {
	case i < int(phaseUnknown):
		return name[i]
	default:
		return name[int(phaseUnknown)]
	}
}

type lineType int

const (
	lineEmpty lineType = iota
	lineSection
	linePolicyDef
	lineUnknown
)

func (t lineType) String() string {
	name := []string{"lineEmpty", "lineSection", "linePolicyDef", "lineUnknown"}
	i := int(t)
	switch {
	case i < int(lineUnknown):
		return name[i]
	default:
		return name[int(lineUnknown)]
	}
}

type lineCtx struct {
	no      int
	origin  string
	trimed  string
	ltype   lineType
	section string
	phs     phase
	service *pms.Service
}

var emptyPS pms.PolicyStore

func (s *Store) readSPDLWithoutLock() (*pms.PolicyStore, error) {
	f, err := os.Open(s.FileLocation)
	if err != nil {
		return &emptyPS, errors.Wrapf(err, errors.StoreError, "unable to open file %q", s.FileLocation)
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Warnf("Unable to close file %s because of error %v", s.FileLocation, err)
		}
	}()

	var firstErr error
	ps := parseSPDL(f, s.FileLocation, func(err error) bool {
		firstErr = err
		return false
	})
	if firstErr != nil {
		return &emptyPS, firstErr
	}
	return ps, nil
}

// parseSPDL reads SPDL from r, named name in errors. It calls onError with
// each error and stops when onError returns false.
func parseSPDL(r io.Reader, name string, onError func(error) bool) *pms.PolicyStore {
	var ps pms.PolicyStore
	lc := lineCtx{}
	br := bufio.NewReader(r)
	for {
		if err := readLine(br, &lc); err != nil {
			if err != io.EOF {
				onError(err)
			}
			break
		}

		var err error
		if err = determineType(&lc); err == nil {
			switch lc.ltype {
			case lineEmpty:
			case lineSection:
				err = processSection(&ps, &lc)
			case linePolicyDef:
				if err = processPolicyDef(&ps, &lc); err != nil {
					err = fmt.Errorf("%s:%d: %w", name, lc.no, err)
				}
			default:
				//lint:ignore ST1005 Preserve the public parser error text for existing callers.
				err = fmt.Errorf("Unknown line type %s", lc.ltype)
			}
		}
		if err != nil && !onError(err) {
			break
		}
	}
	return &ps
}

// CheckSPDL reads the SPDL in r, named name in errors, and returns every
// error in it rather than only the first.
func CheckSPDL(r io.Reader, name string) (*pms.PolicyStore, []error) {
	var errs []error
	ps := parseSPDL(r, name, func(err error) bool {
		errs = append(errs, err)
		return true
	})
	return ps, errs
}

func getServiceSection(ps *pms.PolicyStore, service string) *pms.Service {
	for _, svc := range ps.Services {
		if svc.Name == service {
			return svc
		}
	}
	return nil
}

func processPolicySection(ps *pms.PolicyStore, lc *lineCtx) error {
	// Check if in correct service
	if lc.phs == phaseRoot {
		//lint:ignore ST1005 Preserve the public parser error text for existing callers.
		return fmt.Errorf("Policy section %s is in wrong service section", lc.section)
	}
	lc.phs = phasePolicy

	return nil
}

func processRolePolicySection(ps *pms.PolicyStore, lc *lineCtx) error {
	// Check if in correct service
	if lc.phs == phaseRoot {
		//lint:ignore ST1005 Preserve the public parser error text for existing callers.
		return fmt.Errorf("Policy section %s is in wrong service section", lc.section)
	}
	lc.phs = phaseRolepolicy

	return nil
}

func processServiceSection(ps *pms.PolicyStore, lc *lineCtx) error {
	serviceName := lc.section[len("service."):]
	service := getServiceSection(ps, serviceName)
	if service != nil {
		lc.service = service
		return nil
	}
	lc.service = &pms.Service{
		Name: serviceName,
	}
	lc.phs = phaseService
	ps.Services = append(ps.Services, lc.service)
	return nil
}

func processSection(ps *pms.PolicyStore, lc *lineCtx) error {
	// There are three kinds of sections, service, policy and rolepolicy
	switch {
	case strings.HasPrefix(lc.section, "service."):
		return processServiceSection(ps, lc)
	case lc.section == "policy":
		return processPolicySection(ps, lc)
	case lc.section == "rolepolicy":
		return processRolePolicySection(ps, lc)
	default:
		//lint:ignore ST1005 Preserve the public parser error text for existing callers.
		return fmt.Errorf("Unknown section %s", lc.section)
	}
}

func processPolicyPDL(ps *pms.PolicyStore, lc *lineCtx) error {
	policy, _, err := pdl.ParsePolicy(lc.trimed, "")
	if err != nil {
		return err
	}
	policy.ID = suid.New().String()
	lc.service.Policies = append(lc.service.Policies, policy)
	return nil
}

func processRolePolicyPDL(ps *pms.PolicyStore, lc *lineCtx) error {
	rolePolicy, _, err := pdl.ParseRolePolicy(lc.trimed, "")
	if err != nil {
		return err
	}
	rolePolicy.ID = suid.New().String()
	lc.service.RolePolicies = append(lc.service.RolePolicies, rolePolicy)
	return nil
}

func processPolicyDef(ps *pms.PolicyStore, lc *lineCtx) error {
	switch lc.phs {
	case phasePolicy:
		return processPolicyPDL(ps, lc)
	case phaseRolepolicy:
		return processRolePolicyPDL(ps, lc)
	default:
		//lint:ignore ST1005 Preserve the public parser error text for existing callers.
		return fmt.Errorf("Wrong policy definition at line %d", lc.no)
	}
}

func determineType(lc *lineCtx) error {
	lc.trimed = lc.origin
	// Trim comments
	var quote byte
	for i := 0; i < len(lc.origin); i++ {
		c := lc.origin[i]
		if quote != 0 {
			if c == '\\' && i+1 < len(lc.origin) {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
		} else if c == '\'' || c == '"' {
			quote = c
		} else if c == '#' {
			lc.trimed = lc.origin[:i]
			break
		}
	}

	// Trim spaces
	lc.trimed = strings.TrimSpace(lc.trimed)
	if len(lc.trimed) == 0 || lc.trimed[0] == '#' {
		// blank line
		lc.ltype = lineEmpty
		return nil
	}

	if lc.trimed[0] == ']' {
		//lint:ignore ST1005 Preserve the public parser error text for existing callers.
		return fmt.Errorf("Syntax error near %s at line %d", lc.trimed, lc.no)
	}

	if lc.trimed[0] == '[' {
		if len(lc.trimed) == 2 || lc.trimed[len(lc.trimed)-1] != ']' {
			// This is an error, begin with [, but don't end with ]
			//lint:ignore ST1005 Preserve the public parser error text for existing callers.
			return fmt.Errorf("Syntax error near %s at line %d", lc.trimed, lc.no)
		}
		// length > 2 and wrap with []
		lc.section = lc.trimed[1:(len(lc.trimed) - 1)]
		lc.ltype = lineSection
		return nil
	}

	// Don't start with [, assume this is a line for SPDL
	lc.ltype = linePolicyDef

	return nil
}

func readLine(r *bufio.Reader, lc *lineCtx) error {
	lc.origin = ""
	for {
		bs, isp, err := r.ReadLine()
		if err != nil {
			return err
		}
		lc.origin += string(bs)
		if !isp {
			lc.no++
			break
		}
	}

	return nil
}
