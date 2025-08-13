// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	openfga "github.com/openfga/go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func init() {
	// Initialize logger for all tests
	if logger == nil {
		logOptions := &slog.HandlerOptions{}

		// Optional debug logging.
		if os.Getenv("DEBUG") != "" {
			logOptions.Level = slog.LevelDebug
			logOptions.AddSource = true
		}

		logger = slog.New(slog.NewTextHandler(os.Stdout, logOptions))
		slog.SetDefault(logger)
	}
}

// setupService creates a new ProjectsService with mocked external service APIs.
func setupService() *HandlerService {
	if os.Getenv("DEBUG") == "true" {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
	service := &HandlerService{
		fgaService: FgaService{
			client:      &MockFgaClient{},
			cacheBucket: NewMockKeyValue(),
		},
	}

	return service
}

// TestAccessCheckHandler tests the [accessCheckHandler] function.
func TestAccessCheckHandler(t *testing.T) {
	tests := []struct {
		name           string
		messageData    []byte
		replySubject   string
		setupMocks     func(HandlerService, *MockNatsMsg)
		expectedError  bool
		expectedReply  string
		expectedCalled bool
	}{
		{
			name:         "valid single check request",
			messageData:  []byte("project:123#writer@user:456"),
			replySubject: "reply.subject",
			setupMocks: func(service HandlerService, msg *MockNatsMsg) {
				msg.On("Respond", mock.MatchedBy(func(data []byte) bool {
					// Expect a JSON response with check results
					response := string(data)
					return strings.Contains(response, "project:123#writer@user:456\ttrue")
				})).Return(nil).Once()
				// Create the result map for single request
				resultMap := make(map[string]openfga.BatchCheckSingleResult)
				resultMap["1"] = openfga.BatchCheckSingleResult{
					Allowed: openfga.PtrBool(true),
				}
				service.fgaService.client.(*MockFgaClient).On("BatchCheck", mock.Anything, mock.Anything).Return(&openfga.BatchCheckResponse{
					Result: &resultMap,
				}, nil)
				service.fgaService.cacheBucket.(*MockKeyValue).On("PutString", mock.Anything, mock.Anything, mock.Anything).Return(uint64(0), nil)
			},
			expectedError:  false,
			expectedCalled: true,
		},
		{
			name:         "valid multiple check requests",
			messageData:  []byte("project:123#writer@user:456\nproject:789#viewer@user:456"),
			replySubject: "reply.subject",
			setupMocks: func(service HandlerService, msg *MockNatsMsg) {
				msg.On("Respond", mock.MatchedBy(func(data []byte) bool {
					// Expect response with both check results
					response := string(data)
					return strings.Contains(response, "project:123#writer@user:456\ttrue") &&
						strings.Contains(response, "project:789#viewer@user:456\ttrue")
				})).Return(nil).Once()
				// Create the result map for multiple requests
				resultMap := make(map[string]openfga.BatchCheckSingleResult, 2)
				resultMap["1"] = openfga.BatchCheckSingleResult{
					Allowed: openfga.PtrBool(true),
				}
				resultMap["2"] = openfga.BatchCheckSingleResult{
					Allowed: openfga.PtrBool(true),
				}
				service.fgaService.client.(*MockFgaClient).On("BatchCheck", mock.Anything, mock.Anything).Return(&openfga.BatchCheckResponse{
					Result: &resultMap,
				}, nil)
				// Mock cache operations
				service.fgaService.cacheBucket.(*MockKeyValue).On("Get", mock.Anything, "inv").Return(nil, jetstream.ErrKeyNotFound)
				service.fgaService.cacheBucket.(*MockKeyValue).On("Get", mock.Anything, mock.AnythingOfType("string")).Return(nil, jetstream.ErrKeyNotFound)
				service.fgaService.cacheBucket.(*MockKeyValue).On("Put", mock.Anything, mock.Anything, mock.Anything).Return(uint64(0), nil)
			},
			expectedError:  false,
			expectedCalled: true,
		},
		{
			name:         "invalid check request format",
			messageData:  []byte("invalid-format"),
			replySubject: "reply.subject",
			setupMocks: func(service HandlerService, msg *MockNatsMsg) {
				msg.On("Respond", []byte("failed to extract check requests")).Return(nil).Once()
			},
			expectedReply:  "failed to extract check requests",
			expectedError:  true,
			expectedCalled: true,
		},
		{
			name:         "empty message data",
			messageData:  []byte(""),
			replySubject: "reply.subject",
			setupMocks: func(service HandlerService, msg *MockNatsMsg) {
				msg.On("Respond", []byte("no check requests found")).Return(nil).Once()
			},
			expectedReply:  "no check requests found",
			expectedError:  false,
			expectedCalled: true,
		},
		{
			name:         "no reply subject - should not respond",
			messageData:  []byte("project:123#writer@user:456"),
			replySubject: "",
			setupMocks: func(service HandlerService, msg *MockNatsMsg) {
				// No Respond call expected
				resultMap := make(map[string]openfga.BatchCheckSingleResult)
				resultMap["1"] = openfga.BatchCheckSingleResult{
					Allowed: openfga.PtrBool(true),
				}
				service.fgaService.client.(*MockFgaClient).On("BatchCheck", mock.Anything, mock.Anything).Return(&openfga.BatchCheckResponse{
					Result: &resultMap,
				}, nil)
			},
			expectedError:  false,
			expectedCalled: false,
		},
		{
			name:         "respond error handling",
			messageData:  []byte("invalid-format"),
			replySubject: "reply.subject",
			setupMocks: func(service HandlerService, msg *MockNatsMsg) {
				msg.On("Respond", []byte("failed to extract check requests")).Return(assert.AnError).Once()
			},
			expectedError:  true,
			expectedCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := CreateMockNatsMsg(tt.messageData)
			msg.reply = tt.replySubject

			handlerService := setupService()
			tt.setupMocks(*handlerService, msg)

			// Test that the function doesn't panic
			assert.NotPanics(t, func() {
				err := handlerService.accessCheckHandler(msg)
				if tt.expectedError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})

			// Verify mock expectations
			if tt.expectedCalled {
				msg.AssertExpectations(t)
			} else {
				// Ensure Respond was never called
				msg.AssertNotCalled(t, "Respond")
			}
		})
	}
}
