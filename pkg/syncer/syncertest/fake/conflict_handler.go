// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fake

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"kpt.dev/configsync/pkg/core"
	"kpt.dev/configsync/pkg/remediator/conflict"
	"kpt.dev/configsync/pkg/status"
)

// ConflictHandler is a fake implementation of conflict.Handler.
type ConflictHandler struct{}

// AddConflictError is a fake implementation of AddConflictError of conflict.Handler.
func (h *ConflictHandler) AddConflictError(core.ID, status.ManagementConflictError) {}

// HasConflictError is a fake implementation of HasConflictError of conflict.Handler.
func (h *ConflictHandler) HasConflictError(_ core.ID) bool {
	return false
}

// RemoveConflictError is a fake implementation of the RemoveConflictError of conflict.Handler.
func (h *ConflictHandler) RemoveConflictError(core.ID) {
}

// ConflictErrors is a fake implementation of ConflictErrors of conflict.Handler.
func (h *ConflictHandler) ConflictErrors() []status.ManagementConflictError {
	return nil
}

// HasConflictErrors is a fake implementation of HasConflictErrors of conflict.Handler.
func (h *ConflictHandler) HasConflictErrors() bool {
	return false
}

// ClearConflictErrorsWithKind is a fake implementation of ClearConflictErrorsWithKind of conflict.Handler.
func (h *ConflictHandler) ClearConflictErrorsWithKind(schema.GroupKind) {
}

var _ conflict.Handler = &ConflictHandler{}

// NewConflictHandler initiates a fake implementation of conflict.Handler.
func NewConflictHandler() conflict.Handler {
	return &ConflictHandler{}
}
