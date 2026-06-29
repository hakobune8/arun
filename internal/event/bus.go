// Copyright 2026 AgentOS Authors
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

package event

import (
	"context"
	"fmt"
	"sync"
)

type subscription struct {
	handler Handler
	types   map[Type]bool // nil means all types
}

// InMemoryBus is an in-memory event bus that dispatches events to
// subscribed handlers synchronously.
type InMemoryBus struct {
	mu   sync.RWMutex
	subs []*subscription
}

// NewInMemoryBus creates a new InMemoryBus.
func NewInMemoryBus() *InMemoryBus {
	return &InMemoryBus{}
}

// Publish dispatches an event to all matching subscribers.
func (b *InMemoryBus) Publish(ctx context.Context, e *Event) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, sub := range b.subs {
		if sub.matches(e.Type) {
			if err := sub.handler(ctx, *e); err != nil {
				return fmt.Errorf("event handler: %w", err)
			}
		}
	}
	return nil
}

// Subscribe registers a handler for the given event types.
// If no types are provided, the handler receives all events.
func (b *InMemoryBus) Subscribe(handler Handler, types ...Type) (Unsubscribe, error) {
	if handler == nil {
		return nil, fmt.Errorf("handler cannot be nil")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	sub := &subscription{handler: handler}
	if len(types) > 0 {
		sub.types = make(map[Type]bool)
		for _, t := range types {
			sub.types[t] = true
		}
	}

	b.subs = append(b.subs, sub)

	unsub := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		for i, s := range b.subs {
			if s == sub {
				b.subs = append(b.subs[:i], b.subs[i+1:]...)
				break
			}
		}
	}

	return unsub, nil
}

// Close clears all subscriptions.
func (b *InMemoryBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs = nil
	return nil
}

func (s *subscription) matches(t Type) bool {
	if s.types == nil {
		return true
	}
	return s.types[t]
}
