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

package validate

import (
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
	v1 "kpt.dev/configsync/pkg/api/configmanagement/v1"
	"kpt.dev/configsync/pkg/core/k8sobjects"
	"kpt.dev/configsync/pkg/importer/analyzer/ast"
	"kpt.dev/configsync/pkg/importer/analyzer/validation/hierarchyconfig"
	"kpt.dev/configsync/pkg/kinds"
	"kpt.dev/configsync/pkg/status"
	"kpt.dev/configsync/pkg/validate/fileobjects"
)

var (
	missingGroup = schema.GroupVersionKind{Version: "v1", Kind: "RoleBinding"}
	missingKind  = kinds.RoleBinding().GroupVersion().WithKind("")
	unknownMode  = v1.HierarchyModeType("unknown")
)

func TestHierarchyConfig(t *testing.T) {
	testCases := []struct {
		name     string
		objs     *fileobjects.Tree
		wantErrs status.MultiError
	}{
		{
			name: "Rolebinding allowed",
			objs: &fileobjects.Tree{
				Cluster: []ast.FileObject{
					k8sobjects.ClusterRoleBinding(),
				},
				HierarchyConfigs: []ast.FileObject{
					k8sobjects.HierarchyConfig(
						k8sobjects.HierarchyConfigKind(v1.HierarchyModeDefault, kinds.RoleBinding())),
				},
			},
		},
		{
			name: "Missing Group allowed",
			objs: &fileobjects.Tree{
				Cluster: []ast.FileObject{
					k8sobjects.ClusterRoleBinding(),
				},
				HierarchyConfigs: []ast.FileObject{
					k8sobjects.HierarchyConfig(
						k8sobjects.HierarchyConfigKind(v1.HierarchyModeDefault, missingGroup)),
				},
			},
		},
		{
			name: "Missing Kind not allowed",
			objs: &fileobjects.Tree{
				Cluster: []ast.FileObject{
					k8sobjects.ClusterRoleBinding(),
				},
				HierarchyConfigs: []ast.FileObject{
					k8sobjects.HierarchyConfig(
						k8sobjects.HierarchyConfigKind(v1.HierarchyModeDefault, missingKind)),
				},
			},
			wantErrs: status.FakeMultiError(hierarchyconfig.UnsupportedResourceInHierarchyConfigErrorCode),
		},
		{
			name: "Cluster-scoped objects not allowed",
			objs: &fileobjects.Tree{
				Cluster: []ast.FileObject{
					k8sobjects.ClusterRoleBinding(),
				},
				HierarchyConfigs: []ast.FileObject{
					k8sobjects.HierarchyConfig(
						k8sobjects.HierarchyConfigKind(v1.HierarchyModeDefault, kinds.ClusterRoleBinding())),
				},
			},
			wantErrs: status.FakeMultiError(hierarchyconfig.ClusterScopedResourceInHierarchyConfigErrorCode),
		},
		{
			name: "ConfigManagement objects not allowed",
			objs: &fileobjects.Tree{
				Cluster: []ast.FileObject{
					k8sobjects.ClusterRoleBinding(),
				},
				HierarchyConfigs: []ast.FileObject{
					k8sobjects.HierarchyConfig(
						k8sobjects.HierarchyConfigKind(v1.HierarchyModeDefault, kinds.Sync())),
				},
			},
			wantErrs: status.FakeMultiError(hierarchyconfig.UnsupportedResourceInHierarchyConfigErrorCode),
		},
		{
			name: "Unknown mode not allowed",
			objs: &fileobjects.Tree{
				Cluster: []ast.FileObject{
					k8sobjects.ClusterRoleBinding(),
				},
				HierarchyConfigs: []ast.FileObject{
					k8sobjects.HierarchyConfig(
						k8sobjects.HierarchyConfigKind(unknownMode, kinds.Role())),
				},
			},
			wantErrs: status.FakeMultiError(hierarchyconfig.IllegalHierarchyModeErrorCode),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := HierarchyConfig(tc.objs)
			if !errors.Is(errs, tc.wantErrs) {
				t.Errorf("got HierarchyConfig() error %v, want %v", errs, tc.wantErrs)
			}
		})
	}
}
