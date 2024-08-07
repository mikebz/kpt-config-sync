// Copyright 2022 Google LLC
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

package parse

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"kpt.dev/configsync/pkg/core"
	"kpt.dev/configsync/pkg/core/k8sobjects"
	"kpt.dev/configsync/pkg/declared"
	"kpt.dev/configsync/pkg/importer/analyzer/ast"
	"kpt.dev/configsync/pkg/status"
	"kpt.dev/configsync/pkg/testing/testerrors"
)

func TestNamespaceScopeVisitor(t *testing.T) {
	testCases := []struct {
		name    string
		scope   declared.Scope
		obj     ast.FileObject
		want    ast.FileObject
		wantErr status.Error
	}{
		{
			name:  "correct Namespace pass",
			scope: "foo",
			obj:   k8sobjects.Role(core.Namespace("foo")),
		},
		{
			name:  "blank Namespace pass and update Namespace",
			scope: "foo",
			obj:   k8sobjects.Role(core.Namespace("")),
			want:  k8sobjects.Role(core.Namespace("foo")),
		},
		{
			name:    "wrong Namespace error",
			scope:   "foo",
			obj:     k8sobjects.Role(core.Namespace("bar")),
			wantErr: BadScopeErr(k8sobjects.Role(core.Namespace("bar")), "foo"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.want.Unstructured == nil {
				// We don't expect repositoryScopeVisitor to mutate the object.
				tc.want = tc.obj.DeepCopy()
			}

			visitor := repositoryScopeVisitor(tc.scope)

			_, err := visitor([]ast.FileObject{tc.obj})
			testerrors.AssertEqual(t, tc.wantErr, err)

			if diff := cmp.Diff(tc.want, tc.obj, ast.CompareFileObject); diff != "" {
				// Either the visitor didn't mutate the object, or it unexpectedly did so.
				t.Error(diff)
			}
		})
	}
}
