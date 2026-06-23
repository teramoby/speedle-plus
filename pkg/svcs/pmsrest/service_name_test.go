package pmsrest

import (
	"strings"
	"testing"
)

func TestValidateServiceName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid simple service name",
			input:   "my-service",
			wantErr: false,
		},
		{
			name:    "valid name with dots",
			input:   "com.example.service",
			wantErr: false,
		},
		{
			name:    "valid name with underscores and hyphens",
			input:   "my_service-v2",
			wantErr: false,
		},
		{
			name:    "valid alphanumeric only",
			input:   "Service123",
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
			errMsg:  "service name must be between 1 and 128 characters",
		},
		{
			name:    "name too long (129 characters)",
			input:   strings.Repeat("a", 129),
			wantErr: true,
			errMsg:  "service name must be between 1 and 128 characters",
		},
		{
			name:    "name at max length boundary (128 characters)",
			input:   strings.Repeat("a", 128),
			wantErr: false,
		},
		{
			name:    "name with invalid character (space)",
			input:   "my service",
			wantErr: true,
			errMsg:  "service name contains invalid character",
		},
		{
			name:    "name with invalid character (slash)",
			input:   "my/service",
			wantErr: true,
			errMsg:  "service name contains invalid character",
		},
		{
			name:    "name with invalid character (backslash)",
			input:   `my\service`,
			wantErr: true,
			errMsg:  "service name contains invalid character",
		},
		{
			name:    "name with invalid character (colons)",
			input:   "http://example",
			wantErr: true,
			errMsg:  "service name contains invalid character",
		},
		{
			name:    "name with path traversal dots",
			input:   "../etc/passwd",
			wantErr: true,
			errMsg:  "service name contains invalid character",
		},
		{
			name:    "single character name",
			input:   "a",
			wantErr: false,
		},
		{
			name:    "name with at sign",
			input:   "user@host",
			wantErr: true,
			errMsg:  "service name contains invalid character",
		},
		{
			name:    "name with only dots",
			input:   "a.b.c",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateServiceName(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
