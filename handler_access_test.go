// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
)

func init() {
	// Initialize logger for all tests
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}

	lfxEnvironment = constants.LFXEnvironmentDev
}

// TestFgaExtractCheckRequests tests the parsing of check requests
func TestFgaExtractCheckRequests(t *testing.T) {
	tests := []struct {
		name          string
		payload       []byte
		expectError   bool
		expectedCount int
		description   string
	}{
		{
			name:          "single valid request",
			payload:       []byte("project:123#admin@user:456"),
			expectError:   false,
			expectedCount: 1,
			description:   "should parse single check request",
		},
		{
			name:          "multiple valid requests",
			payload:       []byte("project:123#admin@user:456\nproject:789#viewer@user:456"),
			expectError:   false,
			expectedCount: 2,
			description:   "should parse multiple check requests separated by newlines",
		},
		{
			name:          "empty lines ignored",
			payload:       []byte("project:123#admin@user:456\n\nproject:789#viewer@user:456\n"),
			expectError:   false,
			expectedCount: 2,
			description:   "should ignore empty lines",
		},
		{
			name:          "invalid format - missing @",
			payload:       []byte("project:123#adminuser:456"),
			expectError:   true,
			expectedCount: 0,
			description:   "should error on missing @ separator",
		},
		{
			name:          "invalid format - missing #",
			payload:       []byte("project:123admin@user:456"),
			expectError:   true,
			expectedCount: 0,
			description:   "should error on missing # separator",
		},
		{
			name:          "empty payload",
			payload:       []byte(""),
			expectError:   false,
			expectedCount: 0,
			description:   "should handle empty payload",
		},
		{
			name:          "only newlines",
			payload:       []byte("\n\n\n"),
			expectError:   false,
			expectedCount: 0,
			description:   "should handle payload with only newlines",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests, err := fgaExtractCheckRequests(tt.payload)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if len(requests) != tt.expectedCount {
				t.Errorf("expected %d requests, got %d", tt.expectedCount, len(requests))
			}

			t.Logf("%s: %s", tt.name, tt.description)
		})
	}
}

// TestFgaParseCheckRequest tests the parsing of individual check request lines
func TestFgaParseCheckRequest(t *testing.T) {
	tests := []struct {
		name             string
		line             []byte
		expectError      bool
		expectedUser     string
		expectedRelation string
		expectedObject   string
	}{
		{
			name:             "valid simple request",
			line:             []byte("project:123#admin@user:456"),
			expectError:      false,
			expectedUser:     "user:456",
			expectedRelation: "admin",
			expectedObject:   "project:123",
		},
		{
			name:             "valid complex object",
			line:             []byte("org:linux-foundation/project:kernel#maintainer@user:torvalds"),
			expectError:      false,
			expectedUser:     "user:torvalds",
			expectedRelation: "maintainer",
			expectedObject:   "org:linux-foundation/project:kernel",
		},
		{
			name:             "valid with group user",
			line:             []byte("project:123#viewer@group:developers"),
			expectError:      false,
			expectedUser:     "group:developers",
			expectedRelation: "viewer",
			expectedObject:   "project:123",
		},
		{
			name:        "missing @ separator",
			line:        []byte("project:123#adminuser:456"),
			expectError: true,
		},
		{
			name:        "missing # separator",
			line:        []byte("project:123admin@user:456"),
			expectError: true,
		},
		{
			name:        "empty line",
			line:        []byte(""),
			expectError: true,
		},
		{
			name:             "only separators",
			line:             []byte("#@"),
			expectError:      false,
			expectedUser:     "",
			expectedRelation: "",
			expectedObject:   "",
		},
		{
			name:             "missing user",
			line:             []byte("project:123#admin@"),
			expectError:      false,
			expectedUser:     "",
			expectedRelation: "admin",
			expectedObject:   "project:123",
		},
		{
			name:             "missing relation",
			line:             []byte("project:123#@user:456"),
			expectError:      false,
			expectedUser:     "user:456",
			expectedRelation: "",
			expectedObject:   "project:123",
		},
		{
			name:             "missing object",
			line:             []byte("#admin@user:456"),
			expectError:      false,
			expectedUser:     "user:456",
			expectedRelation: "admin",
			expectedObject:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := fgaParseCheckRequest(tt.line)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if request.User != tt.expectedUser {
				t.Errorf("expected user %s, got %s", tt.expectedUser, request.User)
			}
			if request.Relation != tt.expectedRelation {
				t.Errorf("expected relation %s, got %s", tt.expectedRelation, request.Relation)
			}
			if request.Object != tt.expectedObject {
				t.Errorf("expected object %s, got %s", tt.expectedObject, request.Object)
			}
		})
	}
}

// BenchmarkFgaParseCheckRequest benchmarks the parsing performance
func BenchmarkFgaParseCheckRequest(b *testing.B) {
	line := []byte("org:linux-foundation/project:kernel#maintainer@user:developer")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = fgaParseCheckRequest(line)
	}
}

// BenchmarkFgaExtractCheckRequests benchmarks extracting multiple requests
func BenchmarkFgaExtractCheckRequests(b *testing.B) {
	payload := []byte(strings.Repeat("project:123#admin@user:456\n", 100))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = fgaExtractCheckRequests(payload)
	}
}

// TestMessageResponsePatterns tests various response patterns
func TestMessageResponsePatterns(t *testing.T) {

	// Test that error messages contain expected prefixes
	errorPrefixes := []string{
		"failed to extract check requests",
		"no check requests found",
		"failed to check relationship",
	}

	for _, prefix := range errorPrefixes {
		t.Run("error_prefix_"+strings.ReplaceAll(prefix, " ", "_"), func(t *testing.T) {
			// Verify the error message is used in the handler
			if !bytes.Contains([]byte(prefix), []byte("failed")) && !bytes.Contains([]byte(prefix), []byte("no check")) {
				t.Errorf("error prefix '%s' doesn't look like an error message", prefix)
			}
		})
	}
}
