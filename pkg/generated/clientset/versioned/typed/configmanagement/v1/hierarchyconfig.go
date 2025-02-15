// Code generated by client-gen. DO NOT EDIT.

package v1

import (
	context "context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	gentype "k8s.io/client-go/gentype"
	configmanagementv1 "kpt.dev/configsync/pkg/api/configmanagement/v1"
	scheme "kpt.dev/configsync/pkg/generated/clientset/versioned/scheme"
)

// HierarchyConfigsGetter has a method to return a HierarchyConfigInterface.
// A group's client should implement this interface.
type HierarchyConfigsGetter interface {
	HierarchyConfigs() HierarchyConfigInterface
}

// HierarchyConfigInterface has methods to work with HierarchyConfig resources.
type HierarchyConfigInterface interface {
	Create(ctx context.Context, hierarchyConfig *configmanagementv1.HierarchyConfig, opts metav1.CreateOptions) (*configmanagementv1.HierarchyConfig, error)
	Update(ctx context.Context, hierarchyConfig *configmanagementv1.HierarchyConfig, opts metav1.UpdateOptions) (*configmanagementv1.HierarchyConfig, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*configmanagementv1.HierarchyConfig, error)
	List(ctx context.Context, opts metav1.ListOptions) (*configmanagementv1.HierarchyConfigList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *configmanagementv1.HierarchyConfig, err error)
	HierarchyConfigExpansion
}

// hierarchyConfigs implements HierarchyConfigInterface
type hierarchyConfigs struct {
	*gentype.ClientWithList[*configmanagementv1.HierarchyConfig, *configmanagementv1.HierarchyConfigList]
}

// newHierarchyConfigs returns a HierarchyConfigs
func newHierarchyConfigs(c *ConfigmanagementV1Client) *hierarchyConfigs {
	return &hierarchyConfigs{
		gentype.NewClientWithList[*configmanagementv1.HierarchyConfig, *configmanagementv1.HierarchyConfigList](
			"hierarchyconfigs",
			c.RESTClient(),
			scheme.ParameterCodec,
			"",
			func() *configmanagementv1.HierarchyConfig { return &configmanagementv1.HierarchyConfig{} },
			func() *configmanagementv1.HierarchyConfigList { return &configmanagementv1.HierarchyConfigList{} },
		),
	}
}
