//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/svcs"
)

// The console actions a user may be granted on the console resource.
const (
	ActionRead  = "read"
	ActionWrite = "write"
	// Resource is the resource the console asks ADS about.
	Resource = "console"
)

// Authorizer decides whether principals may perform a console action.
// An error means no decision could be made; callers must then deny.
type Authorizer interface {
	Allowed(ctx context.Context, principals []*ads.Principal, action string) (bool, error)
}

// maxCacheEntries bounds the decision cache; it is cleared when full.
const maxCacheEntries = 10000

type decision struct {
	allowed bool
	expires time.Time
}

// ADSAuthorizer asks ADS whether the principals may perform the action on
// Resource in Service, and remembers answers for TTL.
type ADSAuthorizer struct {
	Client   *http.Client
	Endpoint string // ADS base URL, such as https://ads:6734
	Service  string
	TTL      time.Duration

	mu    sync.Mutex
	cache map[string]decision
	now   func() time.Time
}

type isAllowedRequest struct {
	Subject     isAllowedSubject `json:"subject"`
	ServiceName string           `json:"serviceName"`
	Resource    string           `json:"resource"`
	Action      string           `json:"action"`
}

type isAllowedSubject struct {
	Principals []*ads.Principal `json:"principals"`
}

type isAllowedResponse struct {
	Allowed      bool   `json:"allowed"`
	ErrorMessage string `json:"errorMessage"`
}

func cacheKey(principals []*ads.Principal, action string) string {
	names := make([]string, 0, len(principals))
	for _, p := range principals {
		names = append(names, PrincipalString(p))
	}
	sort.Strings(names)
	return action + "\n" + strings.Join(names, "\n")
}

func (a *ADSAuthorizer) clock() time.Time {
	if a.now != nil {
		return a.now()
	}
	return time.Now()
}

// Allowed asks ADS, or answers from the cache.
func (a *ADSAuthorizer) Allowed(ctx context.Context, principals []*ads.Principal, action string) (bool, error) {
	key := cacheKey(principals, action)
	now := a.clock()
	a.mu.Lock()
	if d, ok := a.cache[key]; ok && now.Before(d.expires) {
		a.mu.Unlock()
		return d.allowed, nil
	}
	a.mu.Unlock()

	allowed, err := a.ask(ctx, principals, action)
	if err != nil {
		return false, err
	}
	if a.TTL > 0 {
		a.mu.Lock()
		if a.cache == nil || len(a.cache) >= maxCacheEntries {
			a.cache = map[string]decision{}
		}
		a.cache[key] = decision{allowed: allowed, expires: now.Add(a.TTL)}
		a.mu.Unlock()
	}
	return allowed, nil
}

func (a *ADSAuthorizer) ask(ctx context.Context, principals []*ads.Principal, action string) (bool, error) {
	body, _ := json.Marshal(isAllowedRequest{
		Subject:     isAllowedSubject{Principals: principals},
		ServiceName: a.Service,
		Resource:    Resource,
		Action:      action,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(a.Endpoint, "/")+svcs.PolicyAtzPath+"is-allowed", bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.Client.Do(req)
	if err != nil {
		return false, fmt.Errorf("ADS is unreachable: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false, fmt.Errorf("failed to read the ADS response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("ADS answered %d: %s", resp.StatusCode, bytes.TrimSpace(raw))
	}
	var result isAllowedResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return false, fmt.Errorf("failed to decode the ADS response: %w", err)
	}
	if result.ErrorMessage != "" {
		// ADS could not evaluate, for example because the service does not
		// exist yet. That is a denial, not an outage.
		log.Warningf("ADS denied console %s: %s", action, result.ErrorMessage)
		return false, nil
	}
	return result.Allowed, nil
}
