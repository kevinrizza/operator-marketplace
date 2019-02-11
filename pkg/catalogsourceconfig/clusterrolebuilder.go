package catalogsourceconfig

import (
	"github.com/operator-framework/operator-marketplace/pkg/apis/marketplace/v1alpha1"
	rbac "k8s.io/api/rbac/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterRoleBuilder builds a new ClusterRole object.
type ClusterRoleBuilder struct {
	clusterRole rbac.ClusterRole
}

// ClusterRole returns a ClusterRole object.
func (b *ClusterRoleBuilder) ClusterRole() *rbac.ClusterRole {
	return &b.clusterRole
}

// WithTypeMeta sets basic TypeMeta.
func (b *ClusterRoleBuilder) WithTypeMeta() *ClusterRoleBuilder {
	b.clusterRole.TypeMeta = meta.TypeMeta{
		Kind:       "ClusterRole",
		APIVersion: "v1",
	}
	return b
}

// WithMeta sets basic TypeMeta and ObjectMeta.
func (b *ClusterRoleBuilder) WithMeta(name string) *ClusterRoleBuilder {
	b.WithTypeMeta()
	if b.clusterRole.GetObjectMeta() == nil {
		b.clusterRole.ObjectMeta = meta.ObjectMeta{}
	}
	b.clusterRole.SetName(name)
	return b
}

// WithOwner sets the owner of the ClusterRole object to the given owner.
func (b *ClusterRoleBuilder) WithOwner(owner *v1alpha1.CatalogSourceConfig) *ClusterRoleBuilder {
	b.clusterRole.SetOwnerReferences(append(b.clusterRole.GetOwnerReferences(),
		[]meta.OwnerReference{
			*meta.NewControllerRef(owner, owner.GroupVersionKind()),
		}[0]))
	return b
}

// WithRules sets the rules for the ClusterRoles
func (b *ClusterRoleBuilder) WithRules(rules []rbac.PolicyRule) *ClusterRoleBuilder {
	b.clusterRole.Rules = rules
	return b
}

// NewClusterRule returns PolicyRule objects
func NewClusterRule(verbs, apiGroups, resources []string) rbac.PolicyRule {
	return rbac.PolicyRule{
		Verbs:     verbs,
		APIGroups: apiGroups,
		Resources: resources,
	}
}
