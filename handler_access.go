// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// The fga-sync service.
package main

import (
	"context"

	nats "github.com/nats-io/nats.go"
)

// accessCheckHandler handles access check requests from the NATS server.
func accessCheckHandler(message *nats.Msg) {
	ctx := context.TODO()

	var response []byte
	var err error

	logger.With("message", string(message.Data)).InfoContext(ctx, "handling access check request")

	// Extract the check requests from the message payload.
	checkRequests, err := fgaExtractCheckRequests(message.Data)
	if err != nil {
		errText := "failed to extract check requests"
		logger.With(errKey, err).WarnContext(ctx, errText)
		if message.Reply != "" {
			// Send a reply if an inbox was provided.
			if err = message.Respond([]byte(errText)); err != nil {
				logger.With(errKey, err).WarnContext(ctx, "failed to send reply")
			}
		}
		return
	}

	if len(checkRequests) == 0 {
		errText := "no check requests found"
		logger.WarnContext(ctx, errText)
		if message.Reply != "" {
			// Send a reply if an inbox was provided.
			if err = message.Respond([]byte(errText)); err != nil {
				logger.With(errKey, err).WarnContext(ctx, "failed to send reply")
			}
		}
		return
	}

	logger.With("count", len(checkRequests)).DebugContext(ctx, "checking fga relationships")
	response, err = fgaCheckRelationships(ctx, checkRequests)
	if err != nil {
		errText := "failed to check relationship"
		logger.With(errKey, err).ErrorContext(ctx, errText)
		if message.Reply != "" {
			// Send a reply if an inbox was provided.
			if err = message.Respond([]byte(errText)); err != nil {
				logger.With(errKey, err).WarnContext(ctx, "failed to send reply")
			}
		}
		return
	}

	if message.Reply != "" {
		// Send a reply if an inbox was provided.
		if err = message.Respond(response); err != nil {
			logger.With(errKey, err).WarnContext(ctx, "failed to send reply")
			return
		}

		logger.With("message", string(message.Data), "response", string(response)).InfoContext(ctx, "sent access check response")
	}
}
