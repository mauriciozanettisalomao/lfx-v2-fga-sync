// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// The fga-sync service.
package main

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
	"github.com/openfga/go-sdk/client" // Only for client types, not the full SDK
)

type meetingStub struct {
	UID        string   `json:"uid"`
	Public     bool     `json:"public"`
	ProjectUID string   `json:"project_uid"`
	Organizers []string `json:"organizers"`
	Committees []string `json:"committees"`
}

// buildMeetingTuples builds all of the tuples for a meeting object.
func (h *HandlerService) buildMeetingTuples(ctx context.Context, object string, meeting *meetingStub) ([]client.ClientTupleKey, error) {
	tuples := h.fgaService.NewTupleKeySlice(4)

	// Convert the "public" attribute to a "user:*" relation.
	if meeting.Public {
		tuples = append(tuples, h.fgaService.TupleKey(constants.UserWildcard, constants.RelationViewer, object))
	}

	// Add the project relation to associate this meeting with its project
	if meeting.ProjectUID != "" {
		tuples = append(tuples, h.fgaService.TupleKey(constants.ObjectTypeProject+meeting.ProjectUID, constants.RelationProject, object))
	}

	// Each committee set on the meeting according to the payload should have a committee relation with the meeting.
	for _, committee := range meeting.Committees {
		tuples = append(tuples, h.fgaService.TupleKey(constants.ObjectTypeCommittee+committee, constants.RelationCommittee, object))
	}

	// Query the project's meeting organizers from OpenFGA to give each project-level meeting
	// organizer the organizer relation with the meeting.
	projectObject := constants.ObjectTypeProject + meeting.ProjectUID
	projectOrganizers, err := h.fgaService.GetTuplesByRelation(ctx, projectObject, constants.RelationMeetingOrganizer)
	if err != nil {
		logger.With(errKey, err, "project", projectObject).WarnContext(ctx, "failed to read project tuples, continuing without project organizers")
		// Continue without project organizers rather than failing the entire update
	} else {
		for _, tuple := range projectOrganizers {
			tuples = append(tuples, h.fgaService.TupleKey(tuple.Key.User, constants.RelationOrganizer, object))
		}
	}

	// Each organizer set on the meeting according to the payload should get the organizer relation.
	for _, principal := range meeting.Organizers {
		tuples = append(tuples, h.fgaService.TupleKey(constants.ObjectTypeUser+principal, constants.RelationOrganizer, object))
	}

	return tuples, nil
}

// meetingUpdateAccessHandler handles meeting access control updates.
func (h *HandlerService) meetingUpdateAccessHandler(message INatsMsg) error {
	ctx := context.Background()

	logger.With("message", string(message.Data())).InfoContext(ctx, "handling project access control update")

	// Parse the event data.
	meeting := new(meetingStub)
	var err error
	err = json.Unmarshal(message.Data(), meeting)
	if err != nil {
		logger.With(errKey, err).ErrorContext(ctx, "event data parse error")
		return err
	}

	// Grab the project ID.
	if meeting.ProjectUID == "" {
		logger.ErrorContext(ctx, "meeting project ID not found")
		return errors.New("meeting project ID not found")
	}

	object := constants.ObjectTypeMeeting + meeting.UID

	// Build a list of tuples to sync.
	//
	// It is important that all tuples that should exist with respect to the meeting object
	// should be added to this tuples list because when SyncObjectTuples is called, it will delete
	// all tuples that are not in the tuples list parameter.
	tuples, err := h.buildMeetingTuples(ctx, object, meeting)
	if err != nil {
		logger.With(errKey, err, "object", object).ErrorContext(ctx, "failed to build meeting tuples")
		return err
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

// meetingDeleteAllAccessHandler handles deleting all tuples for a meeting object.
//
// This should only happen when a meeting is deleted.
func (h *HandlerService) meetingDeleteAllAccessHandler(message INatsMsg) error {
	ctx := context.Background()

	logger.With("message", string(message.Data())).InfoContext(ctx, "handling meeting access control delete all")

	meetingUID := string(message.Data())
	if meetingUID == "" {
		logger.ErrorContext(ctx, "empty deletion payload")
		return errors.New("empty deletion payload")
	}
	if meetingUID[0] == '{' || meetingUID[0] == '[' || meetingUID[0] == '"' {
		// This event payload is not supposed to be serialized.
		logger.ErrorContext(ctx, "unsupported deletion payload")
		return errors.New("unsupported deletion payload")
	}

	object := constants.ObjectTypeMeeting + meetingUID

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

		logger.With("object", object).InfoContext(ctx, "sent meeting access control delete all response")
	}

	return nil
}

type registrantStub struct {
	// UID is the registrant ID for the user's registration on the meeting.
	UID string `json:"uid"`
	// MeetingUID is the meeting ID for the meeting the registrant is registered for.
	MeetingUID string `json:"meeting_uid"`
	// Host determines whether the user should get host relation on the meeting
	Host bool `json:"host"`
}

// meetingRegistrantAddHandler handles adding a registrant to a meeting.
func (h *HandlerService) meetingRegistrantAddHandler(message INatsMsg) error {
	ctx := context.Background()

	logger.With("message", string(message.Data())).InfoContext(ctx, "handling meeting registrant add")

	// Parse the event data.
	registrant := new(registrantStub)
	err := json.Unmarshal(message.Data(), registrant)
	if err != nil {
		logger.With(errKey, err).ErrorContext(ctx, "event data parse error")
		return err
	}

	// Validate required fields.
	if registrant.UID == "" {
		logger.ErrorContext(ctx, "registrant UID not found")
		return errors.New("registrant UID not found")
	}
	if registrant.MeetingUID == "" {
		logger.ErrorContext(ctx, "meeting UID not found")
		return errors.New("meeting UID not found")
	}

	meetingObject := constants.ObjectTypeMeeting + registrant.MeetingUID
	userPrincipal := constants.ObjectTypeUser + registrant.UID

	// Determine the relation based on whether they are a host.
	// Host implies participant, so we only need to add the host relation.
	relation := constants.RelationParticipant
	if registrant.Host {
		relation = constants.RelationHost
	}

	// Write the tuple directly (not using SyncObjectTuples since we're only adding one specific relation).
	err = h.fgaService.WriteTuple(ctx, userPrincipal, relation, meetingObject)
	if err != nil {
		logger.With(errKey, err, "user", userPrincipal, "relation", relation, "meeting", meetingObject).ErrorContext(ctx, "failed to write registrant tuple")
		return err
	}

	logger.With(
		"user", userPrincipal,
		"relation", relation,
		"meeting", meetingObject,
	).InfoContext(ctx, "added registrant to meeting")

	if message.Reply() != "" {
		// Send a reply if an inbox was provided.
		if err = message.Respond([]byte("OK")); err != nil {
			logger.With(errKey, err).WarnContext(ctx, "failed to send reply")
			return err
		}

		logger.With("meeting", meetingObject, "registrant", userPrincipal).InfoContext(ctx, "sent registrant add response")
	}

	return nil
}

// meetingRegistrantRemoveHandler handles removing a registrant from a meeting.
func (h *HandlerService) meetingRegistrantRemoveHandler(message INatsMsg) error {
	ctx := context.Background()

	logger.With("message", string(message.Data())).InfoContext(ctx, "handling meeting registrant remove")

	// Parse the event data.
	registrant := new(registrantStub)
	err := json.Unmarshal(message.Data(), registrant)
	if err != nil {
		logger.With(errKey, err).ErrorContext(ctx, "event data parse error")
		return err
	}

	// Validate required fields.
	if registrant.UID == "" {
		logger.ErrorContext(ctx, "registrant UID not found")
		return errors.New("registrant UID not found")
	}
	if registrant.MeetingUID == "" {
		logger.ErrorContext(ctx, "meeting UID not found")
		return errors.New("meeting UID not found")
	}

	meetingObject := constants.ObjectTypeMeeting + registrant.MeetingUID
	userPrincipal := constants.ObjectTypeUser + registrant.UID

	// Determine the relation to remove based on whether they were a host.
	// If they were a host, remove the host relation. Otherwise, remove participant.
	relation := constants.RelationParticipant
	if registrant.Host {
		relation = constants.RelationHost
	}

	// Delete the tuple directly.
	err = h.fgaService.DeleteTuple(ctx, userPrincipal, relation, meetingObject)
	if err != nil {
		logger.With(errKey, err, "user", userPrincipal, "relation", relation, "meeting", meetingObject).ErrorContext(ctx, "failed to delete registrant tuple")
		return err
	}

	logger.With(
		"user", userPrincipal,
		"relation", relation,
		"meeting", meetingObject,
	).InfoContext(ctx, "removed registrant from meeting")

	if message.Reply() != "" {
		// Send a reply if an inbox was provided.
		if err = message.Respond([]byte("OK")); err != nil {
			logger.With(errKey, err).WarnContext(ctx, "failed to send reply")
			return err
		}

		logger.With("meeting", meetingObject, "registrant", userPrincipal).InfoContext(ctx, "sent registrant remove response")
	}

	return nil
}
