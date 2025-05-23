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

package e2e

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"kpt.dev/configsync/e2e/nomostest"
	"kpt.dev/configsync/e2e/nomostest/gitproviders"
	"kpt.dev/configsync/e2e/nomostest/ntopts"
	nomostesting "kpt.dev/configsync/e2e/nomostest/testing"
	"kpt.dev/configsync/pkg/api/configsync"
	"kpt.dev/configsync/pkg/api/configsync/v1beta1"
	"kpt.dev/configsync/pkg/core/k8sobjects"
	"kpt.dev/configsync/pkg/kinds"
	"kpt.dev/configsync/pkg/metadata"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestProfilingResourcesByObjectCount loops through multiple steps. Each
// step adds and removes incrementally more Deployments. This is useful for
// profiling the resource requests, usage, and limits relative to the number of
// managed objects.
// Skipped by default, because it needs cluster autoscaling and a lot of quota.
func TestProfilingResourcesByObjectCount(t *testing.T) {
	rootSyncID := nomostest.DefaultRootSyncID
	nt := nomostest.New(t, nomostesting.Reconciliation1, ntopts.ProfilingTest,
		ntopts.SyncWithGitSource(rootSyncID, ntopts.Unstructured),
		ntopts.WithReconcileTimeout(configsync.DefaultReconcileTimeout))
	rootSyncGitRepo := nt.SyncSourceGitReadWriteRepository(rootSyncID)

	syncPath := filepath.Join(gitproviders.DefaultSyncDir, "stress-test")
	ns := "stress-test-ns"

	steps := 4
	deploysPerStep := 500

	for step := 1; step <= steps; step++ {
		deployCount := deploysPerStep * step

		nt.T.Logf("Adding a test namespace and %d deployments", deployCount)
		nt.Must(rootSyncGitRepo.Add(fmt.Sprintf("%s/ns-%s.yaml", syncPath, ns), k8sobjects.NamespaceObject(ns)))

		for i := 1; i <= deployCount; i++ {
			name := fmt.Sprintf("pause-%d", i)
			nt.Must(rootSyncGitRepo.Add(
				fmt.Sprintf("%s/namespaces/%s/deployment-%s.yaml", syncPath, ns, name),
				pauseDeploymentObject(nt, name, ns)))
		}

		nt.Must(rootSyncGitRepo.CommitAndPush(fmt.Sprintf("Adding a test namespace and %d deployments", deployCount)))

		// Validate that the resources sync without the reconciler running out of
		// memory, getting OOMKilled, and crash looping.
		nt.Must(nt.WatchForAllSyncs())

		nt.T.Logf("Verify the number of Deployment objects")
		nt.Must(validateNumberOfObjectsEquals(nt, kinds.Deployment(), deployCount,
			client.MatchingLabels{metadata.ManagedByKey: metadata.ManagedByValue},
			client.InNamespace(ns)))

		nt.T.Log("Removing resources from Git")
		nt.Must(rootSyncGitRepo.Remove(syncPath))
		nt.Must(rootSyncGitRepo.CommitAndPush("Removing resources from Git"))

		// Validate that the resources sync without the reconciler running out of
		// memory, getting OOMKilled, and crash looping.
		nt.Must(nt.WatchForAllSyncs())
	}
}

// TestProfilingResourcesByObjectCountWithMultiSync loops through multiple steps.
// Each step provisions multiple RootSyncs, each with many Deployments applied
// and then pruned. Each subsequent step manages more and more Deployments.
// This is useful for profiling the resource requests, usage, and limits
// relative to the number of managed objects.
// Skipped by default, because it needs cluster autoscaling and a lot of quota.
func TestProfilingResourcesByObjectCountWithMultiSync(t *testing.T) {
	rootSyncID := nomostest.DefaultRootSyncID
	nt := nomostest.New(t, nomostesting.Reconciliation1, ntopts.ProfilingTest,
		ntopts.SyncWithGitSource(rootSyncID, ntopts.Unstructured),
		ntopts.WithReconcileTimeout(configsync.DefaultReconcileTimeout))
	rootSyncGitRepo := nt.SyncSourceGitReadWriteRepository(rootSyncID)

	steps := 4
	deploysPerStep := 1000
	syncCount := 10

	// Create the namespace using the default root-sync.
	// Use the same namespace for all other RSyncs to ensure they all show up in watches, whether it's cluster-scope or namespace-scope.
	ns := "stress-test-ns"
	nt.T.Logf("Adding test namespace: %s", ns)
	nt.Must(rootSyncGitRepo.Add(fmt.Sprintf("%s/ns-%s.yaml", gitproviders.DefaultSyncDir, ns), k8sobjects.NamespaceObject(ns)))

	for syncIndex := 1; syncIndex <= syncCount; syncIndex++ {
		syncName := fmt.Sprintf("sync-%d", syncIndex)
		// Use the same Git repo for all the RootSyncs, but have them sync from different directories.
		syncPath := syncName
		syncObj := nomostest.RootSyncObjectV1Beta1FromOtherRootRepo(nt, syncName, configsync.RootSyncName)
		syncObj.Spec.Git.Dir = syncPath
		syncObj.Spec.SafeOverride().LogLevels = []v1beta1.ContainerLogLevelOverride{
			{
				ContainerName: "reconciler",
				LogLevel:      3,
			},
		}
		// Manage the RootSyncs with the parent root-sync
		nt.Must(rootSyncGitRepo.Add(
			fmt.Sprintf("%s/namespaces/%s/rootsync-%s.yaml", gitproviders.DefaultSyncDir, configsync.ControllerNamespace, syncName),
			syncObj))

		// Update sync expectation
		syncID, err := kinds.LookupID(syncObj, nt.Scheme)
		require.NoError(nt.T, err)
		nomostest.SetExpectedSyncPath(nt, syncID, syncPath)
	}

	for step := 1; step <= steps; step++ {
		totalDeployCount := deploysPerStep * step
		nt.T.Logf("Starting step %d: %d Deployments total", step, totalDeployCount)

		for syncIndex := 1; syncIndex <= syncCount; syncIndex++ {
			syncName := fmt.Sprintf("sync-%d", syncIndex)
			syncPath := syncName
			deployCount := totalDeployCount / syncCount

			nt.T.Logf("Adding %d deployments for RootSync %s", deployCount, syncName)

			for deployIndex := 1; deployIndex <= deployCount; deployIndex++ {
				deployName := fmt.Sprintf("pause-%d-%d", syncIndex, deployIndex)
				nt.Must(rootSyncGitRepo.Add(
					fmt.Sprintf("%s/namespaces/%s/deployment-%s.yaml", syncPath, ns, deployName),
					pauseDeploymentObject(nt, deployName, ns)))
			}

			nt.Must(rootSyncGitRepo.CommitAndPush(fmt.Sprintf("Adding %d deployments for %d RootSyncs", deployCount, syncCount)))
		}

		// Validate that the resources sync without the reconciler running out of
		// memory, getting OOMKilled, and crash looping.
		nt.Must(nt.WatchForAllSyncs())

		nt.T.Logf("Verify the number of Deployment objects")
		nt.Must(validateNumberOfObjectsEquals(nt, kinds.Deployment(), totalDeployCount,
			client.MatchingLabels{metadata.ManagedByKey: metadata.ManagedByValue},
			client.InNamespace(ns)))

		nt.T.Log("Removing resources from Git")
		for syncIndex := 1; syncIndex <= syncCount; syncIndex++ {
			syncName := fmt.Sprintf("sync-%d", syncIndex)
			syncPath := syncName
			nt.Must(rootSyncGitRepo.Remove(syncPath))
			// Add an empty sync directory so the reconciler doesn't error that it can't find it.
			nt.Must(rootSyncGitRepo.AddEmptyDir(syncPath))
		}
		nt.Must(rootSyncGitRepo.CommitAndPush("Removing resources from Git"))

		// Validate that the resources sync without the reconciler running out of
		// memory, getting OOMKilled, and crash looping.
		nt.Must(nt.WatchForAllSyncs())

		nt.T.Logf("Verify all Deployments deleted")
		nt.Must(validateNumberOfObjectsEquals(nt, kinds.Deployment(), 0,
			client.MatchingLabels{metadata.ManagedByKey: metadata.ManagedByValue},
			client.InNamespace(ns)))
	}

	nt.T.Log("Removing RootSyncs from Git")
	for syncIndex := 1; syncIndex <= syncCount; syncIndex++ {
		syncName := fmt.Sprintf("sync-%d", syncIndex)
		nt.Must(rootSyncGitRepo.Remove(
			fmt.Sprintf("%s/namespaces/%s/rootsync-%s.yaml", gitproviders.DefaultSyncDir, configsync.ControllerNamespace, syncName)))

		// Remove sync expectation
		syncObj := k8sobjects.RootSyncObjectV1Beta1(syncName)
		syncID, err := kinds.LookupID(syncObj, nt.Scheme)
		require.NoError(nt.T, err)
		delete(nt.SyncSources, syncID)
	}

	nt.T.Logf("Removing test namespace: %s", ns)
	nt.Must(rootSyncGitRepo.Remove(fmt.Sprintf("%s/ns-%s.yaml", gitproviders.DefaultSyncDir, ns)))
	nt.Must(rootSyncGitRepo.CommitAndPush("Removing RootSyncs and test namespace"))

	// Wait for root-sync to sync
	nt.Must(nt.WatchForAllSyncs())
}

// TestProfilingByObjectCountAndSyncCount loops through multiple steps.
// Each step provisions more RootSyncs increasing the number of packages and
// size of the cluster.
// This is useful for showing the exponential reduction in resources from
// watch filtering when adding more packages and increasing the size of the
// cluster.
// Skipped by default, because it needs cluster autoscaling and a lot of quota.
func TestProfilingByObjectCountAndSyncCount(t *testing.T) {
	nt := nomostest.New(t, nomostesting.Reconciliation1, ntopts.ProfilingTest,
		ntopts.SyncWithGitSource(nomostest.DefaultRootSyncID, ntopts.Unstructured),
		ntopts.WithReconcileTimeout(configsync.DefaultReconcileTimeout))
	rootSyncGitRepo := nt.SyncSourceGitReadWriteRepository(nomostest.DefaultRootSyncID)

	steps := 5
	syncsPerStep := 1
	deploysPerSync := 200

	// Create the namespace using the default root-sync.
	// Use the same namespace for all other RSyncs to ensure they all show up in watches, whether it's cluster-scope or namespace-scope.
	ns := "stress-test-ns"
	nt.T.Logf("Adding test namespace: %s", ns)
	nt.Must(rootSyncGitRepo.Add(fmt.Sprintf("%s/ns-%s.yaml", gitproviders.DefaultSyncDir, ns), k8sobjects.NamespaceObject(ns)))

	syncIndex := 0

	for step := 1; step <= steps; step++ {
		syncCount := syncsPerStep * step
		totalDeployCount := syncCount * deploysPerSync
		nt.T.Logf("Starting step %d: %d Syncs with %d Deployments total", step, syncCount, totalDeployCount)

		syncIndex++

		syncName := fmt.Sprintf("sync-%d", syncIndex)
		syncPath := syncName
		deployCount := deploysPerSync

		nt.T.Logf("Adding RootSync %s", syncName)
		syncObj := nomostest.RootSyncObjectV1Beta1FromOtherRootRepo(nt, syncName, configsync.RootSyncName)
		syncObj.Spec.Git.Dir = syncPath
		syncObj.Spec.SafeOverride().LogLevels = []v1beta1.ContainerLogLevelOverride{
			{
				ContainerName: "reconciler",
				LogLevel:      3,
			},
		}
		// Manage the RootSyncs with the parent root-sync
		nt.Must(rootSyncGitRepo.Add(
			fmt.Sprintf("%s/namespaces/%s/rootsync-%s.yaml", gitproviders.DefaultSyncDir, configsync.ControllerNamespace, syncName),
			syncObj))

		nt.T.Logf("Adding %d deployments for RootSync %s", deployCount, syncName)
		for deployIndex := 1; deployIndex <= deployCount; deployIndex++ {
			deployName := fmt.Sprintf("pause-%d-%d", syncIndex, deployIndex)
			nt.Must(rootSyncGitRepo.Add(
				fmt.Sprintf("%s/namespaces/%s/deployment-%s.yaml", syncPath, ns, deployName),
				pauseDeploymentObject(nt, deployName, ns)))
		}

		nt.Must(rootSyncGitRepo.CommitAndPush(fmt.Sprintf("Adding %d deployments each for %d RootSyncs", deployCount, syncCount)))

		// Update sync expectation
		syncID, err := kinds.LookupID(syncObj, nt.Scheme)
		require.NoError(nt.T, err)
		nomostest.SetExpectedSyncPath(nt, syncID, syncPath)

		// Validate that the resources sync without the reconciler running out of
		// memory, getting OOMKilled, and crash looping.
		nt.Must(nt.WatchForAllSyncs())

		nt.T.Logf("Verify the number of Deployment objects")
		nt.Must(validateNumberOfObjectsEquals(nt, kinds.Deployment(), totalDeployCount,
			client.MatchingLabels{metadata.ManagedByKey: metadata.ManagedByValue},
			client.InNamespace(ns)))
	}

	nt.T.Log("Removing RootSyncs from Git")
	for syncIndex := 1; syncIndex <= (syncsPerStep * steps); syncIndex++ {
		syncName := fmt.Sprintf("sync-%d", syncIndex)
		nt.Must(rootSyncGitRepo.Remove(
			fmt.Sprintf("%s/namespaces/%s/rootsync-%s.yaml", gitproviders.DefaultSyncDir, configsync.ControllerNamespace, syncName)))

		// Remove sync expectation
		syncObj := k8sobjects.RootSyncObjectV1Beta1(syncName)
		syncID, err := kinds.LookupID(syncObj, nt.Scheme)
		require.NoError(nt.T, err)
		delete(nt.SyncSources, syncID)
	}

	nt.T.Logf("Removing test namespace: %s", ns)
	nt.Must(rootSyncGitRepo.Remove(fmt.Sprintf("%s/ns-%s.yaml", gitproviders.DefaultSyncDir, ns)))
	nt.Must(rootSyncGitRepo.CommitAndPush("Removing RootSyncs and test namespace"))

	// Wait for root-sync to sync
	nt.Must(nt.WatchForAllSyncs())
}

// TestProfilingResourcesByRootSyncCount applies 10,000 Deployments, across
// 10 different RootSyncs, each managing 1000 Deployments.
// This is useful for profiling the resource requests, usage, and limits
// relative to the total number of objects on the cluster, with a single
// resource type, in the same namespace.
// Skipped by default, because it needs cluster autoscaling and a lot of quota.
func TestProfilingResourcesByRootSyncCount(t *testing.T) {
	nt := nomostest.New(t, nomostesting.Reconciliation1, ntopts.ProfilingTest,
		ntopts.SyncWithGitSource(nomostest.DefaultRootSyncID, ntopts.Unstructured),
		ntopts.WithReconcileTimeout(configsync.DefaultReconcileTimeout))
	rootSyncGitRepo := nt.SyncSourceGitReadWriteRepository(nomostest.DefaultRootSyncID)

	// Create the namespace using the default root-sync.
	// Use the same namespace for all other RSyncs to ensure they all show up in watches, whether it's cluster-scope or namespace-scope.
	ns := "stress-test-ns"
	nt.T.Logf("Adding test namespace: %s", ns)
	nt.Must(rootSyncGitRepo.Add(fmt.Sprintf("%s/ns-%s.yaml", gitproviders.DefaultSyncDir, ns), k8sobjects.NamespaceObject(ns)))

	syncCount := 10
	deployCount := 1000

	for i := 1; i <= syncCount; i++ {
		syncName := fmt.Sprintf("sync-%d", i)
		// Use the same Git repo for all the RootSyncs, but have them sync from different directories.
		syncPath := syncName
		syncObj := nomostest.RootSyncObjectV1Beta1FromOtherRootRepo(nt, syncName, configsync.RootSyncName)
		syncObj.Spec.Git.Dir = syncPath
		syncObj.Spec.SafeOverride().LogLevels = []v1beta1.ContainerLogLevelOverride{
			{
				ContainerName: "reconciler",
				LogLevel:      3,
			},
		}
		// Manage the RootSyncs with the parent root-sync
		nt.Must(rootSyncGitRepo.Add(
			fmt.Sprintf("%s/namespaces/%s/rootsync-%s.yaml", gitproviders.DefaultSyncDir, configsync.ControllerNamespace, syncName),
			syncObj))

		// For each RootSync, make 100 Deployments with unique names
		for j := 1; j <= deployCount; j++ {
			deployName := fmt.Sprintf("pause-%d-%d", i, j)
			nt.Must(rootSyncGitRepo.Add(
				fmt.Sprintf("%s/namespaces/%s/deployment-%s.yaml", syncPath, ns, deployName),
				pauseDeploymentObject(nt, deployName, ns)))
		}

		// Update sync expectation
		syncID, err := kinds.LookupID(syncObj, nt.Scheme)
		require.NoError(nt.T, err)
		nomostest.SetExpectedSyncPath(nt, syncID, syncPath)
	}

	nt.Must(rootSyncGitRepo.CommitAndPush(fmt.Sprintf("Adding %d RootSyncs each with %d deployments", syncCount, deployCount)))

	// Wait for root-sync to sync
	nt.Must(nt.WatchForAllSyncs())

	nt.T.Logf("Verify the number of Deployment objects")
	nt.Must(validateNumberOfObjectsEquals(nt, kinds.Deployment(), syncCount*deployCount,
		client.MatchingLabels{metadata.ManagedByKey: metadata.ManagedByValue},
		client.InNamespace(ns)))

	nt.T.Log("Removing Deployments from Git")
	for i := 1; i <= syncCount; i++ {
		syncPath := fmt.Sprintf("sync-%d", i)
		nt.Must(rootSyncGitRepo.Remove(syncPath))
		// Add an empty sync directory so the reconciler doesn't error that it can't find it.
		nt.Must(rootSyncGitRepo.AddEmptyDir(syncPath))
	}
	nt.Must(rootSyncGitRepo.CommitAndPush("Removing resources from Git"))

	// Validate that the resources sync without the reconciler running out of
	// memory, getting OOMKilled, and crash looping.
	nt.Must(nt.WatchForAllSyncs())

	nt.T.Log("Removing RootSyncs from Git")
	for i := 1; i <= syncCount; i++ {
		syncName := fmt.Sprintf("sync-%d", i)
		nt.Must(rootSyncGitRepo.Remove(
			fmt.Sprintf("%s/namespaces/%s/rootsync-%s.yaml", gitproviders.DefaultSyncDir, configsync.ControllerNamespace, syncName)))

		// Remove sync expectation
		syncObj := k8sobjects.RootSyncObjectV1Beta1(syncName)
		syncID, err := kinds.LookupID(syncObj, nt.Scheme)
		require.NoError(nt.T, err)
		delete(nt.SyncSources, syncID)
	}

	// Validate that the resources sync without the reconciler running out of
	// memory, getting OOMKilled, and crash looping.
	nt.Must(nt.WatchForAllSyncs())
}
