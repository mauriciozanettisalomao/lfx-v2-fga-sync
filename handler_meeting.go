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
func (h *HandlerService) buildMeetingTuples(
	ctx context.Context,
	object string,
	meeting *meetingStub,
) ([]client.ClientTupleKey, error) {
	tuples := h.fgaService.NewTupleKeySlice(4)

	// Convert the "public" attribute to a "user:*" relation.
	if meeting.Public {
		tuples = append(tuples, h.fgaService.TupleKey(constants.UserWildcard, constants.RelationViewer, object))
	}

	// Add the project relation to associate this meeting with its project
	if meeting.ProjectUID != "" {
		tuples = append(
			tuples,
			h.fgaService.TupleKey(constants.ObjectTypeProject+meeting.ProjectUID, constants.RelationProject, object),
			h.fgaService.TupleKey(
				meeting.ProjectUID+"#"+constants.RelationMeetingOrganizer,
				constants.RelationOrganizer,
				object,
			),
		)
	}

	// Each committee set on the meeting according to the payload should have a committee relation with the meeting.
	for _, committee := range meeting.Committees {
		tuples = append(
			tuples,
			h.fgaService.TupleKey(constants.ObjectTypeCommittee+committee, constants.RelationCommittee, object),
			h.fgaService.TupleKey(committee+"#"+constants.RelationMember, constants.RelationParticipant, object),
		)
	}

	// Query the project's meeting organizers from OpenFGA to give each project-level meeting
	// organizer the organizer relation with the meeting.
	projectObject := constants.ObjectTypeProject + meeting.ProjectUID
	projectOrganizers, err := h.fgaService.GetTuplesByRelation(ctx, projectObject, constants.RelationMeetingOrganizer)
	if err != nil {
		logger.WarnContext(
			ctx,
			"failed to read project tuples, continuing without project organizers",
			errKey, err,
			"project", projectObject,
		)
		// Continue without project organizers rather than failing the entire update
	} else {
		for _, tuple := range projectOrganizers {
			tuples = append(tuples, h.fgaService.TupleKey(tuple.Key.User, constants.RelationOrganizer, object))
		}
	}

	// Each organizer set on the meeting according to the payload should get the organizer relation.
	for _, principal := range meeting.Organizers {
		tuples = append(
			tuples,
			h.fgaService.TupleKey(constants.ObjectTypeUser+principal, constants.RelationOrganizer, object),
		)
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
	return h.processDeleteAllAccessMessage(message, constants.ObjectTypeMeeting, "meeting")
}

type registrantStub struct {
	// UID is the registrant ID for the user's registration on the meeting.
	UID string `json:"uid"`
	// MeetingUID is the meeting ID for the meeting the registrant is registered for.
	MeetingUID string `json:"meeting_uid"`
	// Host determines whether the user should get host relation on the meeting
	Host bool `json:"host"`
}

// registrantOperation defines the type of operation to perform on a registrant
type registrantOperation int

const (
	registrantAdd registrantOperation = iota
	registrantRemove
)

// processRegistrantMessage handles the complete message processing flow for registrant operations
func (h *HandlerService) processRegistrantMessage(message INatsMsg, operation registrantOperation) error {
	ctx := context.Background()

	// Log the operation type
	operationType := "add"
	responseMsg := "sent registrant add response"
	if operation == registrantRemove {
		operationType = "remove"
		responseMsg = "sent registrant remove response"
	}

	logger.With("message", string(message.Data())).InfoContext(ctx, "handling meeting registrant "+operationType)

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

	// Perform the FGA operation
	err = h.handleRegistrantOperation(ctx, registrant, operation)
	if err != nil {
		return err
	}

	// Send reply if requested
	if message.Reply() != "" {
		if err = message.Respond([]byte("OK")); err != nil {
			logger.With(errKey, err).WarnContext(ctx, "failed to send reply")
			return err
		}

		logger.InfoContext(ctx, responseMsg,
			"meeting", constants.ObjectTypeMeeting+registrant.MeetingUID,
			"registrant", constants.ObjectTypeUser+registrant.UID,
		)
	}

	return nil
}

// handleRegistrantOperation handles the FGA operation for adding/removing registrants
func (h *HandlerService) handleRegistrantOperation(
	ctx context.Context,
	registrant *registrantStub,
	operation registrantOperation,
) error {
	meetingObject := constants.ObjectTypeMeeting + registrant.MeetingUID
	userPrincipal := constants.ObjectTypeUser + registrant.UID

	// Determine the relation based on whether they are a host
	relation := constants.RelationParticipant
	if registrant.Host {
		relation = constants.RelationHost
	}

	var err error
	var operationName string

	switch operation {
	case registrantAdd:
		operationName = "write"
		err = h.fgaService.WriteTuple(ctx, userPrincipal, relation, meetingObject)
	case registrantRemove:
		operationName = "delete"
		err = h.fgaService.DeleteTuple(ctx, userPrincipal, relation, meetingObject)
	}

	if err != nil {
		logger.ErrorContext(ctx, "failed to "+operationName+" registrant tuple",
			errKey, err,
			"user", userPrincipal,
			"relation", relation,
			"meeting", meetingObject,
		)
		return err
	}

	actionName := "added registrant to"
	if operation == registrantRemove {
		actionName = "removed registrant from"
	}

	logger.With(
		"user", userPrincipal,
		"relation", relation,
		"meeting", meetingObject,
	).InfoContext(ctx, actionName+" meeting")

	return nil
}

// meetingRegistrantAddHandler handles adding a registrant to a meeting.
func (h *HandlerService) meetingRegistrantAddHandler(message INatsMsg) error {
	return h.processRegistrantMessage(message, registrantAdd)
}

// meetingRegistrantRemoveHandler handles removing a registrant from a meeting.
func (h *HandlerService) meetingRegistrantRemoveHandler(message INatsMsg) error {
	return h.processRegistrantMessage(message, registrantRemove)
}
