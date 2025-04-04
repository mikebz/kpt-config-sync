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
	"testing"

	"kpt.dev/configsync/pkg/api/configsync"
	"kpt.dev/configsync/pkg/api/configsync/v1beta1"
	"kpt.dev/configsync/pkg/core/k8sobjects"
	"kpt.dev/configsync/pkg/status"
	"kpt.dev/configsync/pkg/testing/testerrors"
)

func auth(authType configsync.AuthType) func(*v1beta1.RepoSync) {
	return func(sync *v1beta1.RepoSync) {
		sync.Spec.Auth = authType
	}
}

func ociAuth(authType configsync.AuthType) func(*v1beta1.RepoSync) {
	return func(sync *v1beta1.RepoSync) {
		sync.Spec.Oci.Auth = authType
	}
}

func helmAuth(authType configsync.AuthType) func(*v1beta1.RepoSync) {
	return func(sync *v1beta1.RepoSync) {
		sync.Spec.Helm.Auth = authType
	}
}

func named(name string) func(*v1beta1.RepoSync) {
	return func(sync *v1beta1.RepoSync) {
		sync.Name = name
	}
}

func proxy(proxy string) func(*v1beta1.RepoSync) {
	return func(sync *v1beta1.RepoSync) {
		sync.Spec.Proxy = proxy
	}
}

func secret(secretName string) func(*v1beta1.RepoSync) {
	return func(sync *v1beta1.RepoSync) {
		sync.Spec.SecretRef = &v1beta1.SecretReference{
			Name: secretName,
		}
	}
}

func gcpSAEmail(email string) func(sync *v1beta1.RepoSync) {
	return func(sync *v1beta1.RepoSync) {
		sync.Spec.GCPServiceAccountEmail = email
	}
}

func missingRepo(rs *v1beta1.RepoSync) {
	rs.Spec.Repo = ""
}

func missingImage(rs *v1beta1.RepoSync) {
	rs.Spec.Oci.Image = ""
}

func missingHelmRepo(rs *v1beta1.RepoSync) {
	rs.Spec.Helm.Repo = ""
}

func missingHelmChart(rs *v1beta1.RepoSync) {
	rs.Spec.Helm.Chart = ""
}

func repoSyncWithGit(opts ...func(*v1beta1.RepoSync)) *v1beta1.RepoSync {
	rs := k8sobjects.RepoSyncObjectV1Beta1("test-ns", configsync.RepoSyncName)
	rs.Spec.SourceType = configsync.GitSource
	rs.Spec.Git = &v1beta1.Git{
		Repo: "fake repo",
	}
	for _, opt := range opts {
		opt(rs)
	}
	return rs
}

func repoSyncWithOci(opts ...func(*v1beta1.RepoSync)) *v1beta1.RepoSync {
	rs := k8sobjects.RepoSyncObjectV1Beta1("test-ns", configsync.RepoSyncName)
	rs.Spec.SourceType = configsync.OciSource
	rs.Spec.Oci = &v1beta1.Oci{
		Image: "fake image",
	}
	for _, opt := range opts {
		opt(rs)
	}
	return rs
}

func repoSyncWithHelm(opts ...func(*v1beta1.RepoSync)) *v1beta1.RepoSync {
	rs := k8sobjects.RepoSyncObjectV1Beta1("test-ns", configsync.RepoSyncName)
	rs.Spec.SourceType = configsync.HelmSource
	rs.Spec.Helm = &v1beta1.HelmRepoSync{HelmBase: v1beta1.HelmBase{
		Repo:  "fake repo",
		Chart: "fake chart",
	}}
	for _, opt := range opts {
		opt(rs)
	}
	return rs
}

func withGit() func(*v1beta1.RepoSync) {
	return func(sync *v1beta1.RepoSync) {
		sync.Spec.Git = &v1beta1.Git{}
	}
}

func withOci() func(*v1beta1.RepoSync) {
	return func(sync *v1beta1.RepoSync) {
		sync.Spec.Oci = &v1beta1.Oci{}
	}
}

func withHelm() func(*v1beta1.RepoSync) {
	return func(sync *v1beta1.RepoSync) {
		sync.Spec.Helm = &v1beta1.HelmRepoSync{}
	}
}

func rootSyncWithHelm(opts ...func(*v1beta1.RootSync)) *v1beta1.RootSync {
	rs := k8sobjects.RootSyncObjectV1Beta1(configsync.RootSyncName)
	rs.Spec.SourceType = configsync.HelmSource
	rs.Spec.Helm = &v1beta1.HelmRootSync{HelmBase: v1beta1.HelmBase{
		Repo:  "fake repo",
		Chart: "fake chart",
	}}
	for _, opt := range opts {
		opt(rs)
	}
	return rs
}

func helmNsAndDeployNS() func(*v1beta1.RootSync) {
	return func(sync *v1beta1.RootSync) {
		sync.Spec.Helm.Namespace = "test-ns"
		sync.Spec.Helm.DeployNamespace = "test-ns"
	}
}

func TestValidateRepoSyncSpec(t *testing.T) {
	testCases := []struct {
		name    string
		obj     *v1beta1.RepoSync
		wantErr status.Error
	}{
		// Validate Git Spec
		{
			name: "valid git",
			obj:  repoSyncWithGit(auth(configsync.AuthNone)),
		},
		{
			name: "a user-defined name",
			obj:  repoSyncWithGit(auth(configsync.AuthNone), named("user-defined-repo-sync-name")),
		},
		{
			name:    "missing git repo",
			obj:     repoSyncWithGit(auth(configsync.AuthNone), missingRepo),
			wantErr: MissingGitRepo(repoSyncWithGit(auth(configsync.AuthNone), missingRepo)),
		},
		{
			name:    "invalid git auth type",
			obj:     repoSyncWithGit(auth("invalid auth")),
			wantErr: InvalidGitAuthType(repoSyncWithGit(auth("invalid auth"))),
		},
		{
			name:    "no op proxy",
			obj:     repoSyncWithGit(auth(configsync.AuthGCENode), proxy("no-op proxy")),
			wantErr: NoOpProxy(repoSyncWithGit(auth(configsync.AuthGCENode), proxy("no-op proxy"))),
		},
		{
			name: "valid proxy with none auth type",
			obj:  repoSyncWithGit(auth(configsync.AuthNone), proxy("ok proxy")),
		},
		{
			name: "valid proxy with cookiefile",
			obj:  repoSyncWithGit(auth(configsync.AuthCookieFile), secret("cookiefile"), proxy("ok proxy")),
		},
		{
			name: "valid proxy with token",
			obj:  repoSyncWithGit(auth(configsync.AuthToken), secret("token"), proxy("ok proxy")),
		},
		{
			name:    "illegal secret",
			obj:     repoSyncWithGit(auth(configsync.AuthNone), secret("illegal secret")),
			wantErr: IllegalSecretRef(configsync.GitSource, repoSyncWithGit(auth(configsync.AuthNone), secret("illegal secret"))),
		},
		{
			name:    "missing secret",
			obj:     repoSyncWithGit(auth(configsync.AuthSSH)),
			wantErr: MissingSecretRef(configsync.GitSource, repoSyncWithGit(auth(configsync.AuthSSH))),
		},
		{
			name:    "invalid GCP serviceaccount email",
			obj:     repoSyncWithGit(auth(configsync.AuthGCPServiceAccount), gcpSAEmail("invalid_gcp_sa@gserviceaccount.com")),
			wantErr: InvalidGCPSAEmail(configsync.GitSource, repoSyncWithGit(auth(configsync.AuthGCPServiceAccount), gcpSAEmail("invalid_gcp_sa@gserviceaccount.com"))),
		},
		{
			name:    "invalid GCP serviceaccount email with correct suffix",
			obj:     repoSyncWithGit(auth(configsync.AuthGCPServiceAccount), gcpSAEmail("foo@my-project.iam.gserviceaccount.com")),
			wantErr: InvalidGCPSAEmail(configsync.GitSource, repoSyncWithGit(auth(configsync.AuthGCPServiceAccount), gcpSAEmail("foo@my-project.iam.gserviceaccount.com"))),
		},
		{
			name:    "invalid GCP serviceaccount email without domain",
			obj:     repoSyncWithGit(auth(configsync.AuthGCPServiceAccount), gcpSAEmail("my-project")),
			wantErr: InvalidGCPSAEmail(configsync.GitSource, repoSyncWithGit(auth(configsync.AuthGCPServiceAccount), gcpSAEmail("my-project"))),
		},
		{
			name:    "missing GCP serviceaccount email for git",
			obj:     repoSyncWithGit(auth(configsync.AuthGCPServiceAccount)),
			wantErr: MissingGCPSAEmail(configsync.GitSource, repoSyncWithGit(auth(configsync.AuthGCPServiceAccount))),
		},
		// Validate OCI spec
		{
			name: "valid oci",
			obj:  repoSyncWithOci(ociAuth(configsync.AuthNone)),
		},
		{
			name:    "missing oci image",
			obj:     repoSyncWithOci(ociAuth(configsync.AuthNone), missingImage),
			wantErr: MissingOciImage(repoSyncWithOci(ociAuth(configsync.AuthNone), missingImage)),
		},
		{
			name:    "invalid auth type",
			obj:     repoSyncWithOci(ociAuth("invalid auth")),
			wantErr: InvalidOciAuthType(repoSyncWithOci(ociAuth("invalid auth"))),
		},
		{
			name:    "missing GCP serviceaccount email for Oci",
			obj:     repoSyncWithOci(ociAuth(configsync.AuthGCPServiceAccount)),
			wantErr: MissingGCPSAEmail(configsync.OciSource, repoSyncWithOci(ociAuth(configsync.AuthGCPServiceAccount))),
		},
		{
			name:    "invalid source type",
			obj:     k8sobjects.RepoSyncObjectV1Beta1("test-ns", configsync.RepoSyncName, k8sobjects.WithRepoSyncSourceType("invalid")),
			wantErr: InvalidSourceType(k8sobjects.RepoSyncObjectV1Beta1("test-ns", configsync.RepoSyncName, k8sobjects.WithRepoSyncSourceType("invalid"))),
		},
		{
			name:    "redundant OCI spec",
			obj:     repoSyncWithGit(withOci()),
			wantErr: InvalidGitAuthType(repoSyncWithGit(withOci())),
		},
		{
			name:    "redundant Git spec",
			obj:     repoSyncWithOci(withGit()),
			wantErr: InvalidOciAuthType(repoSyncWithOci(withGit())),
		},
		// Validate Helm spec
		{
			name: "valid helm",
			obj:  repoSyncWithHelm(helmAuth(configsync.AuthNone)),
		},
		{
			name:    "missing helm repo",
			obj:     repoSyncWithHelm(helmAuth(configsync.AuthNone), missingHelmRepo),
			wantErr: MissingHelmRepo(repoSyncWithHelm(helmAuth(configsync.AuthNone), missingHelmRepo)),
		},
		{
			name:    "missing helm chart",
			obj:     repoSyncWithHelm(helmAuth(configsync.AuthNone), missingHelmChart),
			wantErr: MissingHelmChart(repoSyncWithHelm(helmAuth(configsync.AuthNone), missingHelmChart)),
		},
		{
			name:    "invalid auth type",
			obj:     repoSyncWithHelm(helmAuth("invalid auth")),
			wantErr: InvalidHelmAuthType(repoSyncWithHelm(helmAuth("invalid auth"))),
		},
		{
			name:    "missing GCP serviceaccount email for Helm",
			obj:     repoSyncWithHelm(helmAuth(configsync.AuthGCPServiceAccount)),
			wantErr: MissingGCPSAEmail(configsync.HelmSource, repoSyncWithHelm(helmAuth(configsync.AuthGCPServiceAccount))),
		},
		{
			name:    "redundant Helm spec",
			obj:     repoSyncWithGit(withHelm()),
			wantErr: InvalidGitAuthType(repoSyncWithGit(withHelm())),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := RepoSyncSpec(tc.obj.Spec.SourceType, tc.obj.Spec.Git, tc.obj.Spec.Oci, tc.obj.Spec.Helm, tc.obj)
			testerrors.AssertEqual(t, tc.wantErr, err)
		})
	}
}

func TestValidateRootSyncSpec(t *testing.T) {
	testCases := []struct {
		name    string
		obj     *v1beta1.RootSync
		wantErr status.Error
	}{
		// spec.helm.namespace and spec.helm.deployNamespace are mutually exclusive
		{
			name:    "valid git",
			obj:     rootSyncWithHelm(helmNsAndDeployNS()),
			wantErr: InvalidHelmAuthType(rootSyncWithHelm(helmNsAndDeployNS())),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := RootSyncSpec(tc.obj.Spec.SourceType, tc.obj.Spec.Git, tc.obj.Spec.Oci, tc.obj.Spec.Helm, tc.obj)
			testerrors.AssertEqual(t, tc.wantErr, err)
		})
	}
}
