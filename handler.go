// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import nats "github.com/nats-io/nats.go"

// HandlerService is the service that handles the messages from NATS about FGA syncing.
type HandlerService struct {
	fgaService FgaService
}

// INatsMsg is an interface for [nats.Msg] that allows for mocking.
type INatsMsg interface {
	Reply() string
	Respond(data []byte) error
	Data() []byte
	Subject() string
}

// NatsMsg is a wrapper around [nats.Msg] that implements [INatsMsg].
type NatsMsg struct {
	*nats.Msg
}

// Reply implements [INatsMsg.Reply].
func (m *NatsMsg) Reply() string {
	return m.Msg.Reply
}

// Respond implements [INatsMsg.Respond].
func (m *NatsMsg) Respond(data []byte) error {
	return m.Msg.Respond(data)
}

// Data implements [INatsMsg.Data].
func (m *NatsMsg) Data() []byte {
	return m.Msg.Data
}

// Subject implements [INatsMsg.Subject].
func (m *NatsMsg) Subject() string {
	return m.Msg.Subject
}
