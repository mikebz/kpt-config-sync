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

package watch

import (
	"context"
	"errors"
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"kpt.dev/configsync/pkg/applyset"
	"kpt.dev/configsync/pkg/declared"
	"kpt.dev/configsync/pkg/metadata"
	"kpt.dev/configsync/pkg/remediator/conflict"
	"kpt.dev/configsync/pkg/remediator/queue"
	"kpt.dev/configsync/pkg/status"
	syncerclient "kpt.dev/configsync/pkg/syncer/client"
)

// Manager accepts new resource lists that are parsed from Git and then
// updates declared resources and get GVKs.
type Manager struct {
	// scope is the scope of the reconciler process running the Manager.
	scope declared.Scope

	// syncName is the corresponding RootSync|RepoSync name of the reconciler process running the Manager.
	syncName string

	// cfg is the rest config used to talk to apiserver.
	cfg *rest.Config

	// resources is the declared resources that are parsed from Git.
	resources *declared.Resources

	// queue is the work queue for remediator.
	queue *queue.ObjectQueue

	// watcherFactory is the function to create a watcher.
	watcherFactory watcherFactory

	// mapper is the RESTMapper used by the watcherFactory
	mapper meta.RESTMapper

	// labelSelector filters watches
	labelSelector labels.Selector

	// The following fields are guarded by the mutex.
	mux sync.RWMutex
	// watching is true if UpdateWatches has been called
	watching bool
	// watcherMap maps GVKs to their associated watchers
	watcherMap map[schema.GroupVersionKind]Runnable
	// needsUpdate indicates if the Manager's watches need to be updated.
	needsUpdate     bool
	conflictHandler conflict.Handler
}

// Options contains options for creating a watch manager.
type Options struct {
	watcherFactory watcherFactory
	mapper         meta.RESTMapper
}

// DefaultOptions return the default options with a ListerWatcherFactory built
// from the specified REST config.
func DefaultOptions(cfg *rest.Config) (*Options, error) {
	factory, err := DynamicListerWatcherFactoryFromConfig(cfg)
	if err != nil {
		return nil, status.APIServerError(err, "failed to build ListerWatcherFactory")
	}

	return &Options{
		watcherFactory: watcherFactoryFromListerWatcherFactory(factory.ListerWatcher),
		mapper:         factory.Mapper,
	}, nil
}

// NewManager starts a new watch manager
func NewManager(scope declared.Scope, syncName string, cfg *rest.Config,
	q *queue.ObjectQueue, decls *declared.Resources, options *Options, ch conflict.Handler) (*Manager, error) {
	if options == nil {
		var err error
		options, err = DefaultOptions(cfg)
		if err != nil {
			return nil, err
		}
	}

	// Only watch & remediate objects applied by this reconciler
	labelSelector := labels.Set{
		metadata.ApplySetPartOfLabel: applyset.IDFromSync(syncName, scope),
	}.AsSelector()

	return &Manager{
		scope:           scope,
		syncName:        syncName,
		cfg:             cfg,
		resources:       decls,
		watcherMap:      make(map[schema.GroupVersionKind]Runnable),
		watcherFactory:  options.watcherFactory,
		mapper:          options.mapper,
		labelSelector:   labelSelector,
		queue:           q,
		conflictHandler: ch,
	}, nil
}

// Watching returns true if UpdateWatches has been called, starting the watchers. This function is threadsafe.
func (m *Manager) Watching() bool {
	m.mux.RLock()
	defer m.mux.RUnlock()
	return m.watching
}

// NeedsUpdate returns true if the Manager's watches need to be updated. This function is threadsafe.
func (m *Manager) NeedsUpdate() bool {
	m.mux.RLock()
	defer m.mux.RUnlock()
	return m.needsUpdate
}

// AddWatches accepts a map of GVKs that should be watched and takes the
// following actions:
//   - start watchers for any GroupVersionKind that is present in the given map
//     and not present in the current watch map.
//
// This function is threadsafe.
func (m *Manager) AddWatches(ctx context.Context, gvkMap map[schema.GroupVersionKind]struct{}) status.MultiError {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.watching = true

	klog.V(3).Infof("AddWatches(%v)", gvkMap)

	var startedWatches uint64

	// Start new watchers
	var errs status.MultiError
	for gvk := range gvkMap {
		// Skip watchers that are already started
		if _, isWatched := m.watcherMap[gvk]; isWatched {
			continue
		}
		// Only start watcher if the resource exists.
		// Watch will be started later by UpdateWatches, after the applier succeeds.
		// TODO: register pending resources and start watching them when the CRD is established
		if _, err := m.mapper.RESTMapping(gvk.GroupKind(), gvk.Version); err != nil {
			switch {
			case meta.IsNoMatchError(err):
				klog.Infof("Remediator skipped adding watch for resource %v: %v: resource watch will be started after apply is successful", gvk, err)
			default:
				errs = status.Append(errs, status.APIServerErrorWrap(err))
			}
			continue
		}
		// We don't have a watcher for this type, so add a watcher for it.
		if err := m.startWatcher(ctx, gvk); err != nil {
			errs = status.Append(errs, err)
			continue
		}
		startedWatches++
	}

	if startedWatches > 0 {
		klog.Infof("The remediator made new progress: started %d new watches", startedWatches)
	} else {
		klog.V(4).Infof("The remediator made no new progress")
	}
	return errs
}

// UpdateWatches accepts a map of GVKs that should be watched and takes the
// following actions:
//   - stop watchers for any GroupVersionKind that is not present in the given
//     map.
//   - start watchers for any GroupVersionKind that is present in the given map
//     and not present in the current watch map.
//
// This function is threadsafe.
func (m *Manager) UpdateWatches(ctx context.Context, gvkMap map[schema.GroupVersionKind]struct{}) status.MultiError {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.watching = true

	klog.V(3).Infof("UpdateWatches(%v)", gvkMap)

	m.needsUpdate = false

	var startedWatches, stoppedWatches uint64
	// Stop obsolete watchers.
	for gvk := range m.watcherMap {
		if _, keepWatching := gvkMap[gvk]; !keepWatching {
			// We were watching the type, but no longer have declarations for it.
			// It is safe to stop the watcher.
			m.stopWatcher(gvk)
			stoppedWatches++
		}
	}

	// Start new watchers
	var errs status.MultiError
	for gvk := range gvkMap {
		// Skip watchers that are already started
		if _, isWatched := m.watcherMap[gvk]; isWatched {
			continue
		}
		// Only start watcher if the resource exists.
		// All resources should exist and be established by now. If not, error.
		if _, err := m.mapper.RESTMapping(gvk.GroupKind(), gvk.Version); err != nil {
			switch {
			case meta.IsNoMatchError(err):
				errs = status.Append(errs, syncerclient.ConflictWatchResourceDoesNotExist(err, gvk))
			default:
				errs = status.Append(errs, status.APIServerErrorWrap(err))
			}
			continue
		}
		// We don't have a watcher for this type, so add a watcher for it.
		if err := m.startWatcher(ctx, gvk); err != nil {
			errs = status.Append(errs, err)
			continue
		}
		startedWatches++
	}

	if startedWatches > 0 || stoppedWatches > 0 {
		klog.Infof("The remediator made new progress: started %d new watches, and stopped %d watches", startedWatches, stoppedWatches)
	} else {
		klog.V(4).Infof("The remediator made no new progress")
	}
	return errs
}

// watchedGVKs returns a list of all GroupVersionKinds currently being watched.
func (m *Manager) watchedGVKs() []schema.GroupVersionKind {
	var gvks []schema.GroupVersionKind
	for gvk := range m.watcherMap {
		gvks = append(gvks, gvk)
	}
	return gvks
}

// startWatcher starts a watcher for a GVK. This function is NOT threadsafe;
// caller must have a lock on m.mux.
func (m *Manager) startWatcher(ctx context.Context, gvk schema.GroupVersionKind) error {
	_, found := m.watcherMap[gvk]
	if found {
		// The watcher is already started.
		return nil
	}
	cfg := watcherConfig{
		gvk:             gvk,
		config:          m.cfg,
		resources:       m.resources,
		queue:           m.queue,
		scope:           m.scope,
		syncName:        m.syncName,
		conflictHandler: m.conflictHandler,
		labelSelector:   m.labelSelector,
	}
	w, err := m.watcherFactory(cfg)
	if err != nil {
		return err
	}

	m.watcherMap[gvk] = w
	go m.runWatcher(ctx, w, gvk)
	return nil
}

// runWatcher blocks until the given watcher finishes running. This function is
// threadsafe.
func (m *Manager) runWatcher(ctx context.Context, r Runnable, gvk schema.GroupVersionKind) {
	if err := r.Run(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			klog.Infof("Watcher stopped for %s: %v", gvk, status.FormatSingleLine(err))
		} else {
			klog.Warningf("Error running watcher for %s: %v", gvk, status.FormatSingleLine(err))
		}
		m.mux.Lock()
		delete(m.watcherMap, gvk)
		m.needsUpdate = true
		m.mux.Unlock()
	}
}

// stopWatcher stops a watcher for a GVK. This function is NOT threadsafe;
// caller must have a lock on m.mux.
func (m *Manager) stopWatcher(gvk schema.GroupVersionKind) {
	w, found := m.watcherMap[gvk]
	if !found {
		// The watcher is already stopped.
		return
	}

	// Stop the watcher.
	w.Stop()
	// Remove all conflict errors for objects with the same GK because the
	// objects are no longer managed by the reconciler.
	w.ClearManagementConflictsWithKind(gvk.GroupKind())
	delete(m.watcherMap, gvk)
}
