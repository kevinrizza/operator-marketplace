package catalogsourceconfig

import (
	"github.com/operator-framework/operator-marketplace/pkg/apis/marketplace/v1alpha1"
	rbac "k8s.io/api/rbac/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterRoleBindingBuilder builds a new ClusterRoleBinding object.
type ClusterRoleBindingBuilder struct {
	crb rbac.ClusterRoleBinding
}

// ClusterRoleBinding returns a ClusterRoleBinding object.
func (b *ClusterRoleBindingBuilder) ClusterRoleBinding() *rbac.ClusterRoleBinding {
	return &b.crb
}

// WithTypeMeta sets basic TypeMeta.
func (b *ClusterRoleBindingBuilder) WithTypeMeta() *ClusterRoleBindingBuilder {
	b.crb.TypeMeta = meta.TypeMeta{
		Kind:       "ClusterRoleBinding",
		APIVersion: "v1",
	}
	return b
}

// WithMeta sets basic TypeMeta and ObjectMeta.
func (b *ClusterRoleBindingBuilder) WithMeta(name string) *ClusterRoleBindingBuilder {
	b.WithTypeMeta()
	if b.crb.GetObjectMeta() == nil {
		b.crb.ObjectMeta = meta.ObjectMeta{}
	}
	b.crb.SetName(name)
	return b
}

// WithOwner sets the owner of the ClusterRoleBinding object to the given owner.
func (b *ClusterRoleBindingBuilder) WithOwner(owner *v1alpha1.CatalogSourceConfig) *ClusterRoleBindingBuilder {
	b.crb.SetOwnerReferences(append(b.crb.GetOwnerReferences(),
		[]meta.OwnerReference{
			*meta.NewControllerRef(owner, owner.GroupVersionKind()),
		}[0]))
	return b
}

// WithSubjects sets the Subjects for the ClusterRoleBinding
func (b *ClusterRoleBindingBuilder) WithSubjects(subjects []rbac.Subject) *ClusterRoleBindingBuilder {
	b.crb.Subjects = subjects
	return b
}

// WithClusterRoleRef sets the rules for the ClusterRoleBinding
func (b *ClusterRoleBindingBuilder) WithClusterRoleRef(ClusterRoleName string) *ClusterRoleBindingBuilder {
	b.crb.RoleRef = NewClusterRoleRef(ClusterRoleName)
	return b
}

// NewClusterRoleRef returns a new ClusterRoleRef object
func NewClusterRoleRef(ClusterRoleName string) rbac.RoleRef {
	return rbac.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "ClusterRole",
		Name:     ClusterRoleName,
	}
}
