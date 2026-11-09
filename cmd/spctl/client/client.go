//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/teramoby/speedle-plus/pkg/httputils"
)

type Client struct {
	PMSEndpoint string
	HTTPClient  *http.Client
}

func setAuthorizationHeader(req *http.Request, token string) {
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func validateTokenTransport(u *url.URL, token string) error {
	if token == "" || strings.EqualFold(u.Scheme, "https") {
		return nil
	}
	if strings.EqualFold(u.Scheme, "http") {
		hostname := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
		if zone := strings.LastIndex(hostname, "%"); zone >= 0 {
			hostname = hostname[:zone]
		}
		if hostname == "localhost" {
			return nil
		}
		if ip := net.ParseIP(hostname); ip != nil && ip.IsLoopback() {
			return nil
		}
		return fmt.Errorf("refusing to send an API token over insecure HTTP to non-loopback host %q; use an https:// endpoint", u.Hostname())
	}
	return errors.New("API tokens require an https:// endpoint (plain HTTP is allowed only for loopback hosts)")
}

// do also validates redirect targets. net/http otherwise forwards sensitive
// headers on some same-site redirects, including an HTTPS-to-HTTP downgrade.
func (c *Client) do(req *http.Request, token string) (*http.Response, error) {
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if token != "" {
		clone := *httpClient
		checkRedirect := httpClient.CheckRedirect
		clone.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if err := validateTokenTransport(req.URL, token); err != nil {
				return err
			}
			if checkRedirect != nil {
				return checkRedirect(req, via)
			}
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		}
		httpClient = &clone
	}
	return httpClient.Do(req)
}

func redact(value, token string) string {
	if token == "" {
		return value
	}
	return strings.ReplaceAll(value, token, "[redacted]")
}

func responseError(resp *http.Response, paths []string, token string) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var errorDetail httputils.ErrorResponse
	_ = json.Unmarshal(body, &errorDetail)
	code := errorDetail.Error
	retryAfter := redact(resp.Header.Get("Retry-After"), token)
	withRetry := func(message string) string {
		if retryAfter != "" {
			return message + "; Retry-After: " + retryAfter
		}
		return message
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized && code != "":
		return fmt.Errorf("authentication failed (%s): check the API token and endpoint", resp.Status)
	case resp.StatusCode == http.StatusForbidden && code == "insufficient_scope":
		return fmt.Errorf("authorization failed (%s): the API token needs pms:read for reads or pms:write for writes", resp.Status)
	case resp.StatusCode == http.StatusNotFound && (code == "not_found" || code == "service_not_found" || code == "policy_not_found"):
		return fmt.Errorf("requested resource not found (%s)", resp.Status)
	case resp.StatusCode == http.StatusConflict && code == "service_exists":
		return fmt.Errorf("service already exists (%s)", resp.Status)
	case resp.StatusCode == http.StatusConflict && code == "policy_exists":
		return fmt.Errorf("policy already exists (%s)", resp.Status)
	case resp.StatusCode == http.StatusTooManyRequests && code == "daily_quota_exceeded":
		return errors.New(withRetry(fmt.Sprintf("daily quota exceeded (%s): retry after the quota resets", resp.Status)))
	case resp.StatusCode == http.StatusTooManyRequests && strings.Contains(code, "quota"):
		return errors.New(withRetry(fmt.Sprintf("quota exceeded (%s): reduce policy usage before retrying", resp.Status)))
	case resp.StatusCode == http.StatusTooManyRequests:
		return errors.New(withRetry(fmt.Sprintf("request rate limited (%s): retry later with bounded backoff", resp.Status)))
	case code == "quota_unavailable":
		return fmt.Errorf("quota service unavailable (%s): retry later with bounded backoff", resp.Status)
	case strings.Contains(code, "quota"):
		return fmt.Errorf("quota exceeded (%s): reduce policy usage before retrying", resp.Status)
	}

	switch resp.StatusCode {
	case http.StatusNotFound:
		if resp.Request != nil && resp.Request.Method == http.MethodGet {
			return fmt.Errorf("%s not found", strings.Join(paths, " "))
		}
		return errors.New(resp.Status)
	case http.StatusUnauthorized, http.StatusForbidden:
		// Preserve the guidance used by a self-hosted PMS with legacy auth.
		fmt.Println("Authentication or authorization failed. Please specify correct token using '--token' flag.")
		return errors.New(resp.Status)
	case http.StatusBadRequest:
		if code != "" {
			return fmt.Errorf("%s: %s", resp.Status, redact(code, token))
		}
		return errors.New(resp.Status)
	default:
		return errors.New(resp.Status)
	}
}

func (c *Client) delete(u *url.URL, token string) error {
	if err := validateTokenTransport(u, token); err != nil {
		return err
	}
	req, err := http.NewRequest("DELETE", u.String(), nil)
	if err != nil {
		return err
	}

	setAuthorizationHeader(req, token)
	resp, err := c.do(req, token)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return responseError(resp, nil, token)
}

func (c *Client) Delete(paths []string, token string) error {
	u, err := c.pmsURL(paths)
	if err != nil {
		return err
	}
	return c.delete(u, token)
}

func (c *Client) get(u *url.URL, paths []string, params url.Values, token string) ([]byte, error) {
	if err := validateTokenTransport(u, token); err != nil {
		return nil, err
	}
	if params != nil {
		q := u.Query()
		for name, val := range params {
			q.Set(name, val[0])
		}
		u.RawQuery = q.Encode()
	}

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	setAuthorizationHeader(req, token)
	resp, err := c.do(req, token)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return body, nil
	}
	return nil, responseError(resp, paths, token)
}

func (c *Client) Get(paths []string, params url.Values, token string) ([]byte, error) {
	u, err := c.pmsURL(paths)
	if err != nil {
		return nil, err
	}
	return c.get(u, paths, params, token)
}

func (c *Client) post(u *url.URL, paths []string, payload io.Reader, token string) (string, error) {
	if err := validateTokenTransport(u, token); err != nil {
		return "", err
	}
	req, err := http.NewRequest("POST", u.String(), payload)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	setAuthorizationHeader(req, token)
	resp, err := c.do(req, token)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		return string(body), nil
	}
	return "", responseError(resp, paths, token)
}

func (c *Client) Post(paths []string, payload io.Reader, token string) (string, error) {
	u, err := c.pmsURL(paths)
	if err != nil {
		return "", err
	}
	return c.post(u, paths, payload, token)
}

func getURL(baseURL string, paths []string) (*url.URL, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		fmt.Println("Fail to parse URL", err)
		return nil, err
	}
	for i := range paths {
		if len(paths[i]) > 0 {
			u.Path = path.Join(u.Path, paths[i])
		}
	}
	return u, nil
}

func (c *Client) pmsURL(paths []string) (*url.URL, error) {
	return getURL(c.PMSEndpoint, paths)
}
