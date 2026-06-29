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

package memory

import "context"

// Store defines the interface for agent memory persistence.
// Implementations can use vector search, file storage, or SQLite.
// Runtime does not know which implementation is in use.
type Store interface {
	Save(ctx context.Context, entry *Entry) error
	Search(ctx context.Context, query string, limit int) ([]Entry, error)
	Clear(ctx context.Context) error
	Type() string
}
