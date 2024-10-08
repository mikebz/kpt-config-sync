// Copyright 2024 Google LLC
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

package registryproviders

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"kpt.dev/configsync/e2e/nomostest/testkubeclient"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/kyaml/copyutil"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	"sigs.k8s.io/yaml"
)

// use an auto-incrementing index to create unique file names for tarballs
var helmIndex int

type helmOptions struct {
	version      string
	sourceChart  string
	chartObjects []client.Object
	scheme       *runtime.Scheme
}

// HelmOption is an optional parameter when building a helm chart.
type HelmOption func(options *helmOptions)

// HelmChartVersion builds the chart with the specified version.
func HelmChartVersion(version string) func(options *helmOptions) {
	return func(options *helmOptions) {
		options.version = version
	}
}

// HelmSourceChart builds the chart with the specified source chart.
// It should be a subfolder under '../testdata/helm-charts'.
func HelmSourceChart(chart string) func(options *helmOptions) {
	return func(options *helmOptions) {
		options.sourceChart = chart
	}
}

// HelmChartObjects builds the chart with the specified objects.
// A scheme must be provided to encode the object.
func HelmChartObjects(scheme *runtime.Scheme, objs ...client.Object) func(options *helmOptions) {
	return func(options *helmOptions) {
		options.scheme = scheme
		options.chartObjects = objs
	}
}

// BuildHelmPackage creates a new helm chart using the RSync NAMESPACE-NAME as
// the name of the chart. Chart version defaults to "v1.0.0-latest" if not
// specified. The resulting HelmPackage will only exist locally.
func BuildHelmPackage(artifactDir string, provider HelmRegistryProvider, rsRef types.NamespacedName, opts ...HelmOption) (*HelmPackage, error) {
	options := helmOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	name := rsRef.Namespace + "-" + rsRef.Name
	// Use a floating tag when a semver is not specified
	version := options.version
	if version == "" {
		version = "v1.0.0-latest"
	}
	chartName := options.sourceChart
	if chartName == "" {
		chartName = "test"
	}
	// Use chart name/version for context and helmIndex to enforce file name uniqueness.
	// This avoids file name collision even if the test builds an image twice with
	// a dirty repo state.
	tmpDir := filepath.Join(artifactDir, chartName, version, strconv.Itoa(helmIndex))
	if options.sourceChart != "" {
		inputDir := "../testdata/helm-charts/" + options.sourceChart
		if err := copyutil.CopyDir(filesys.MakeFsOnDisk(), inputDir, tmpDir); err != nil {
			return nil, fmt.Errorf("copying package directory: %v", err)
		}
	}
	for _, obj := range options.chartObjects {
		fullPath := filepath.Join(tmpDir, "templates", fmt.Sprintf("%s-%s-%s-%s-%s.yaml",
			obj.GetObjectKind().GroupVersionKind().Group,
			obj.GetObjectKind().GroupVersionKind().Version,
			obj.GetObjectKind().GroupVersionKind().Kind,
			obj.GetNamespace(), obj.GetName()))
		bytes, err := testkubeclient.SerializeObject(obj, ".yaml", options.scheme)
		if err != nil {
			return nil, err
		}
		if err = testkubeclient.WriteToFile(fullPath, bytes); err != nil {
			return nil, err
		}
	}

	// Ensure tmpDir always exists, even if it is an empty chart.
	if err := os.MkdirAll(tmpDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("creating tmp dir: %w", err)
	}
	updateFn := func(chartMap map[string]interface{}) error {
		chartMap["name"] = name
		chartMap["version"] = version
		return nil
	}
	if err := updateYAMLFile(filepath.Join(tmpDir, "Chart.yaml"), updateFn); err != nil {
		return nil, fmt.Errorf("updating Chart.yaml: %v", err)
	}
	packagePath := tmpDir + string(filepath.Separator)
	helmIndex++
	if _, err := provider.Client().Helm("package", tmpDir, "--destination", packagePath); err != nil {
		return nil, fmt.Errorf("packaging helm chart: %w", err)
	}
	helmPackage := &HelmPackage{
		HelmChartID: HelmChartID{
			Name:    name,
			Version: version,
		},
		LocalChartTgzPath: filepath.Join(tmpDir, fmt.Sprintf("%s-%s.tgz", name, version)),
		Provider:          provider,
	}
	return helmPackage, nil
}

func updateYAMLFile(name string, updateFn func(map[string]interface{}) error) error {
	chartBytes, err := os.ReadFile(name)
	if os.IsNotExist(err) {
		chartBytes = []byte{}
	} else if err != nil {
		return fmt.Errorf("reading file: %s: %w", name, err)
	}
	chartManifest := make(map[string]interface{})
	if err := yaml.Unmarshal(chartBytes, &chartManifest); err != nil {
		return fmt.Errorf("parsing yaml file: %s: %w", name, err)
	}
	if err := updateFn(chartManifest); err != nil {
		return fmt.Errorf("updating yaml map for %s: %w", name, err)
	}
	chartBytes, err = yaml.Marshal(chartManifest)
	if err != nil {
		return fmt.Errorf("formatting yaml for %s: %w", name, err)
	}
	if err := os.WriteFile(name, chartBytes, os.ModePerm); err != nil {
		return fmt.Errorf("writing file: %s: %w", name, err)
	}
	return nil
}

// HelmChartID identifies a helm chart.
type HelmChartID struct {
	Name    string
	Version string
}

// String returns NAME/VERSION.
func (id HelmChartID) String() string {
	return fmt.Sprintf("%s/%s", id.Name, id.Version)
}

// HelmPackage represents a helm package that is pushed to a remote registry by the
// test scaffolding. It uses git references as version tags to enable straightforward
// integration with the git e2e tooling and to mimic how a user might leverage
// git and helm.
type HelmPackage struct {
	HelmChartID
	LocalChartTgzPath string
	Digest            string
	Provider          HelmRegistryProvider
}

// Delete the image from the remote registry.
func (h *HelmPackage) Delete() error {
	// How to delete images varies by provider, so delegate deletion to the provider.
	return h.Provider.DeletePackage(h.Name, h.Digest)
}
