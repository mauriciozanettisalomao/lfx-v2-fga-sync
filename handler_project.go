// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// The fga-sync service.
package main

import (
	"context"
	"encoding/json"

	nats "github.com/nats-io/nats.go"
)

type projectStub struct {
	UID       string   `json:"uid"`
	Public    bool     `json:"public"`
	ParentUID string   `json:"parent_uid"`
	Writers   []string `json:"writers"`
	Auditors  []string `json:"auditors"`
}

// projectUpdateAccessHandler handles project access control updates.
func projectUpdateAccessHandler(message *nats.Msg) {
	ctx := context.TODO()

	logger.With("message", string(message.Data)).InfoContext(ctx, "handling project access control update")

	// Parse the event data.
	project := new(projectStub)
	var err error
	err = json.Unmarshal(message.Data, project)
	if err != nil {
		logger.With(errKey, err).ErrorContext(ctx, "event data parse error")
		return
	}

	// Grab the project ID.
	if project.UID == "" {
		logger.ErrorContext(ctx, "project ID not found")
		return
	}

	object := "project:" + project.UID

	// Build a list of tuples to sync.
	tuples := fgaNewTupleKeySlice(4)

	// Convert the "public" attribute to a "user:*" relation.
	if project.Public {
		tuples = append(tuples, fgaTupleKey("user:*", "viewer", object))
	}

	// Handle the parent relation.
	if project.ParentUID != "" {
		tuples = append(tuples, fgaTupleKey("project:"+project.ParentUID, "parent", object))
	}

	// Add each principal from the object as the corresponding relationship tuple
	// (as defined in the OpenFGA schema).
	for _, principal := range project.Writers {
		tuples = append(tuples, fgaTupleKey("user:"+principal, "writer", object))
	}
	for _, principal := range project.Auditors {
		tuples = append(tuples, fgaTupleKey("user:"+principal, "auditor", object))
	}

	tuplesWrites, tuplesDeletes, err := fgaSyncObjectTuples(ctx, object, tuples)
	if err != nil {
		logger.With(errKey, err, "tuples", tuples, "object", object).ErrorContext(ctx, "failed to sync tuples")
		return
	}

	logger.With("tuples", tuples, "object", object, "writes", tuplesWrites, "deletes", tuplesDeletes).InfoContext(ctx, "synced tuples")

	if message.Reply != "" {
		// Send a reply if an inbox was provided.
		if err = message.Respond([]byte("OK")); err != nil {
			logger.With(errKey, err).WarnContext(ctx, "failed to send reply")
			return
		}

		logger.With("object", object).InfoContext(ctx, "sent project access control update response")
	}
}

// projectDeleteAllAccessHandler handles project access control deletions.
func projectDeleteAllAccessHandler(message *nats.Msg) {
	ctx := context.TODO()

	logger.With("message", string(message.Data)).InfoContext(ctx, "handling project access control delete all")

	projectUID := string(message.Data)
	if projectUID == "" {
		logger.ErrorContext(ctx, "empty deletion payload")
		return
	}
	if projectUID[0] == '{' || projectUID[0] == '[' || projectUID[0] == '"' {
		// This event payload is not supposed to be serialized.
		logger.ErrorContext(ctx, "unsupported deletion payload")
		return
	}

	object := "project:" + projectUID

	// Since this is a delete, we can call fgaSyncObjectRelationships directly
	// with a zero-value (nil) slice.
	tuplesWrites, tuplesDeletes, err := fgaSyncObjectTuples(ctx, object, nil)
	if err != nil {
		logger.With(errKey, err, "object", object).ErrorContext(ctx, "failed to sync tuples")
		return
	}

	logger.With("object", object, "writes", tuplesWrites, "deletes", tuplesDeletes).InfoContext(ctx, "synced tuples")

	if message.Reply != "" {
		// Send a reply if an inbox was provided.
		if err = message.Respond([]byte("OK")); err != nil {
			logger.With(errKey, err).WarnContext(ctx, "failed to send reply")
			return
		}

		logger.With("object", object).InfoContext(ctx, "sent project access control delete all response")
	}
}
