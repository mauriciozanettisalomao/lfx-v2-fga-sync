// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// The fga-sync service.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
)

type groupsIOServiceStub struct {
	UID        string              `json:"uid"`
	ObjectType string              `json:"object_type"`
	Public     bool                `json:"public"`
	Relations  map[string][]string `json:"relations"`
	References map[string]string   `json:"references"`
}

// groupsIOServiceUpdateAccessHandler handles groups.io service access control updates.
func (h *HandlerService) groupsIOServiceUpdateAccessHandler(message INatsMsg) error {
	ctx := context.Background()

	logger.With("message", string(message.Data())).InfoContext(ctx, "handling groups.io service access control update")

	// Parse the event data.
	groupsIOService := new(groupsIOServiceStub)
	var err error
	err = json.Unmarshal(message.Data(), groupsIOService)
	if err != nil {
		logger.With(errKey, err).ErrorContext(ctx, "event data parse error")
		return err
	}

	if groupsIOService.UID == "" {
		logger.ErrorContext(ctx, "groups.io service ID not found")
		return errors.New("groups.io service ID not found")
	}

	object := fmt.Sprintf("%s:%s", groupsIOService.ObjectType, groupsIOService.UID)

	// Build a list of tuples to sync.
	tuples := h.fgaService.NewTupleKeySlice(4)

	// Convert the "public" attribute to a "user:*" relation.
	if groupsIOService.Public {
		tuples = append(tuples, h.fgaService.TupleKey(constants.UserWildcard, constants.RelationViewer, object))
	}

	// for parent relation, project relation, etc
	for reference, value := range groupsIOService.References {
		refType := reference
		if reference == constants.RelationParent {
			refType = groupsIOService.ObjectType
		}

		key := fmt.Sprintf("%s:%s", refType, value)
		tuples = append(tuples, h.fgaService.TupleKey(key, reference, object))
	}

	// Add each principal from the object as the corresponding relationship tuple
	// (as defined in the OpenFGA schema).
	// for writer, auditor etc
	for relation, principals := range groupsIOService.Relations {
		for _, principal := range principals {
			tuples = append(tuples, h.fgaService.TupleKey(constants.ObjectTypeUser+principal, relation, object))
		}
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

		logger.With("object", object).InfoContext(ctx, "sent groups.io service access control update response")
	}

	return nil
}

// groupsIOServiceDeleteAllAccessHandler handles groups.io service access control deletions.
func (h *HandlerService) groupsIOServiceDeleteAllAccessHandler(message INatsMsg) error {
	return h.processDeleteAllAccessMessage(message, constants.ObjectTypeGroupsIOService, "groupsio_service")
}
