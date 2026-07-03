// Copyright 2026 ARUN Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package event provides a structured event bus for runtime observability.
// Every runtime operation produces typed events that can be persisted,
// streamed, or replayed.
package event

import (
	"context"
	"time"
)

// Type identifies the kind of event.
type Type string

// All event types in the system.
const (
	TypeTaskCreated      Type = "task.created"
	TypePlanningStarted  Type = "planning.started"
	TypePlanningFinished Type = "planning.finished"
	TypeToolStarted      Type = "tool.started"
	TypeToolFinished     Type = "tool.finished"
	TypeToolFailed       Type = "tool.failed"
	TypeReviewStarted    Type = "review.started"
	TypeReviewFinished   Type = "review.finished"
	TypeRunCompleted     Type = "run.completed"
	TypeRunFailed        Type = "run.failed"
)

// Event is a structured event emitted by the runtime.
type Event struct {
	ID        string      `json:"id"`
	Type      Type        `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	RunID     string      `json:"run_id,omitempty"`
	AgentID   string      `json:"agent_id,omitempty"`
	Data      interface{} `json:"data,omitempty"`
}

// Handler processes an event. Return an error to signal a handler failure,
// but the bus will continue delivering to other handlers.
type Handler func(ctx context.Context, e Event) error

// Bus is the central event bus for runtime observability.
// It allows publishing events and subscribing handlers to event types.
type Bus interface {
	// Publish emits an event to all subscribed handlers.
	Publish(ctx context.Context, e *Event) error

	// Subscribe registers a handler for the given event types.
	// An empty types slice subscribes to all events.
	Subscribe(handler Handler, types ...Type) (Unsubscribe, error)

	// Close shuts down the bus and waits for all handlers to finish.
	Close() error
}

// Unsubscribe cancels a subscription.
type Unsubscribe func()
