// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// The fga-sync service.
package main

import (
	"context"
	"encoding/json"
	"errors"
)

// TODO: update this payload schema to come from the project service
type projectStub struct {
	UID       string   `json:"uid"`
	Public    bool     `json:"public"`
	ParentUID string   `json:"parent_uid"`
	Writers   []string `json:"writers"`
	Auditors  []string `json:"auditors"`
}

// projectUpdateAccessHandler handles project access control updates.
func (h *HandlerService) projectUpdateAccessHandler(message INatsMsg) error {
	ctx := context.Background()

	logger.With("message", string(message.Data())).InfoContext(ctx, "handling project access control update")

	// Parse the event data.
	project := new(projectStub)
	var err error
	err = json.Unmarshal(message.Data(), project)
	if err != nil {
		logger.With(errKey, err).ErrorContext(ctx, "event data parse error")
		return err
	}

	// Grab the project ID.
	if project.UID == "" {
		logger.ErrorContext(ctx, "project ID not found")
		return errors.New("project ID not found")
	}

	object := "project:" + project.UID

	// Build a list of tuples to sync.
	tuples := h.fgaService.NewTupleKeySlice(4)

	// Convert the "public" attribute to a "user:*" relation.
	if project.Public {
		tuples = append(tuples, h.fgaService.TupleKey("user:*", "viewer", object))
	}

	// Handle the parent relation.
	if project.ParentUID != "" {
		tuples = append(tuples, h.fgaService.TupleKey("project:"+project.ParentUID, "parent", object))
	}

	// Add each principal from the object as the corresponding relationship tuple
	// (as defined in the OpenFGA schema).
	for _, principal := range project.Writers {
		tuples = append(tuples, h.fgaService.TupleKey("user:"+principal, "writer", object))
	}
	for _, principal := range project.Auditors {
		tuples = append(tuples, h.fgaService.TupleKey("user:"+principal, "auditor", object))
	}

	tuplesWrites, tuplesDeletes, err := h.fgaService.SyncObjectTuples(ctx, object, tuples)
	if err != nil {
		logger.With(errKey, err, "tuples", tuples, "object", object).ErrorContext(ctx, "failed to sync tuples")
		return err
	}

	logger.With(
		"tuples", tuples,
		"object", object,
		"writes", tuplesWrites,
		"deletes", tuplesDeletes,
	).InfoContext(ctx, "synced tuples")

	if message.Reply() != "" {
		// Send a reply if an inbox was provided.
		if err = message.Respond([]byte("OK")); err != nil {
			logger.With(errKey, err).WarnContext(ctx, "failed to send reply")
			return err
		}

		logger.With("object", object).InfoContext(ctx, "sent project access control update response")
	}

	return nil
}

// projectDeleteAllAccessHandler handles project access control deletions.
func (h *HandlerService) projectDeleteAllAccessHandler(message INatsMsg) error {
	ctx := context.Background()

	logger.With("message", string(message.Data())).InfoContext(ctx, "handling project access control delete all")

	projectUID := string(message.Data())
	if projectUID == "" {
		logger.ErrorContext(ctx, "empty deletion payload")
		return errors.New("empty deletion payload")
	}
	if projectUID[0] == '{' || projectUID[0] == '[' || projectUID[0] == '"' {
		// This event payload is not supposed to be serialized.
		logger.ErrorContext(ctx, "unsupported deletion payload")
		return errors.New("unsupported deletion payload")
	}

	object := "project:" + projectUID

	// Since this is a delete, we can call fgaSyncObjectRelationships directly
	// with a zero-value (nil) slice.
	tuplesWrites, tuplesDeletes, err := h.fgaService.SyncObjectTuples(ctx, object, nil)
	if err != nil {
		logger.With(errKey, err, "object", object).ErrorContext(ctx, "failed to sync tuples")
		return err
	}

	logger.With("object", object, "writes", tuplesWrites, "deletes", tuplesDeletes).InfoContext(ctx, "synced tuples")

	if message.Reply() != "" {
		// Send a reply if an inbox was provided.
		if err = message.Respond([]byte("OK")); err != nil {
			logger.With(errKey, err).WarnContext(ctx, "failed to send reply")
			return err
		}

		logger.With("object", object).InfoContext(ctx, "sent project access control delete all response")
	}

	return nil
}
