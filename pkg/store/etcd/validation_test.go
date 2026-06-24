//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package etcd

import (
	"testing"
	"time"
)

func TestValidateServiceName(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		wantErr     bool
		errContains string
	}{
		{
			name:        "empty name",
			serviceName: "",
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "contains slash",
			serviceName: "my/service",
			wantErr:     true,
			errContains: "cannot contain",
		},
		{
			name:        "starts with slash",
			serviceName: "/badservice",
			wantErr:     true,
			errContains: "cannot contain",
		},
		{
			name:        "ends with slash",
			serviceName: "badservice/",
			wantErr:     true,
			errContains: "cannot contain",
		},
		{
			name:        "multiple slashes",
			serviceName: "a/b/c",
			wantErr:     true,
			errContains: "cannot contain",
		},
		{
			name:        "only slash",
			serviceName: "/",
			wantErr:     true,
			errContains: "cannot contain",
		},
		{
			name:        "valid simple name",
			serviceName: "my-service",
			wantErr:     false,
		},
		{
			name:        "valid name with dots",
			serviceName: "my.service.name",
			wantErr:     false,
		},
		{
			name:        "valid name with hyphens and underscores",
			serviceName: "my-service_v2",
			wantErr:     false,
		},
		{
			name:        "valid single character",
			serviceName: "a",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateServiceName(tt.serviceName)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateServiceName(%q) error = %v, wantErr = %v", tt.serviceName, err, tt.wantErr)
				return
			}
			if err != nil && tt.errContains != "" {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("validateServiceName(%q) error = %v, expected to contain %q", tt.serviceName, err, tt.errContains)
				}
			}
		})
	}
}

func TestComputeWatchBackoff(t *testing.T) {
	tests := []struct {
		name             string
		consecutiveFails int
		expectedMin      time.Duration
		expectedMax      time.Duration
	}{
		{
			name:             "first failure",
			consecutiveFails: 1,
			expectedMin:      1 * time.Second,
			expectedMax:      1 * time.Second,
		},
		{
			name:             "second failure",
			consecutiveFails: 2,
			expectedMin:      2 * time.Second,
			expectedMax:      2 * time.Second,
		},
		{
			name:             "third failure",
			consecutiveFails: 3,
			expectedMin:      4 * time.Second,
			expectedMax:      4 * time.Second,
		},
		{
			name:             "fourth failure",
			consecutiveFails: 4,
			expectedMin:      8 * time.Second,
			expectedMax:      8 * time.Second,
		},
		{
			name:             "fifth failure",
			consecutiveFails: 5,
			expectedMin:      16 * time.Second,
			expectedMax:      16 * time.Second,
		},
		{
			name:             "sixth failure hits max",
			consecutiveFails: 6,
			expectedMin:      30 * time.Second,
			expectedMax:      30 * time.Second,
		},
		{
			name:             "beyond max stays capped",
			consecutiveFails: 10,
			expectedMin:      30 * time.Second,
			expectedMax:      30 * time.Second,
		},
		{
			name:             "zero failures",
			consecutiveFails: 0,
			expectedMin:      1 * time.Second,
			expectedMax:      1 * time.Second,
		},
		{
			name:             "negative failures treated as zero",
			consecutiveFails: -1,
			expectedMin:      1 * time.Second,
			expectedMax:      1 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeWatchBackoff(tt.consecutiveFails)
			if got < tt.expectedMin || got > tt.expectedMax {
				t.Errorf("computeWatchBackoff(%d) = %v, want [%v, %v]",
					tt.consecutiveFails, got, tt.expectedMin, tt.expectedMax)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
