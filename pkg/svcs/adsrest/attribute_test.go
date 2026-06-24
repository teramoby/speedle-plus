package adsrest

import (
	"strings"
	"testing"
)

func TestVerifyAttributeName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid simple attribute name",
			input:   "userName",
			wantErr: false,
		},
		{
			name:    "valid name with underscores",
			input:   "user_name",
			wantErr: false,
		},
		{
			name:    "valid name with hyphens",
			input:   "x-forwarded-for",
			wantErr: false,
		},
		{
			name:    "valid name with dots",
			input:   "request.path",
			wantErr: false,
		},
		{
			name:    "valid name with mixed separators",
			input:   "my-attr_name.v2",
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
			errMsg:  "attribute name must not be empty",
		},
		{
			name:    "name too long (257 characters)",
			input:   strings.Repeat("a", 257),
			wantErr: true,
			errMsg:  "attribute name must be at most 256 characters",
		},
		{
			name:    "name at max length boundary (256 characters)",
			input:   strings.Repeat("a", 256),
			wantErr: false,
		},
		{
			name:    "name with space",
			input:   "user name",
			wantErr: true,
			errMsg:  "attribute name contains invalid character",
		},
		{
			name:    "name with slash",
			input:   "user/name",
			wantErr: true,
			errMsg:  "attribute name contains invalid character",
		},
		{
			name:    "name with backslash",
			input:   `path\to\attr`,
			wantErr: true,
			errMsg:  "attribute name contains invalid character",
		},
		{
			name:    "name with colon",
			input:   "ns:attr",
			wantErr: true,
			errMsg:  "attribute name contains invalid character",
		},
		{
			name:    "name with at sign",
			input:   "attr@domain",
			wantErr: true,
			errMsg:  "attribute name contains invalid character",
		},
		{
			name:    "single character name",
			input:   "x",
			wantErr: false,
		},
		{
			name:    "numeric name",
			input:   "12345",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyAttributeName(tt.input)
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
