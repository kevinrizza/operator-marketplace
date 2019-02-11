package catalogsourceconfig

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/operator-framework/operator-marketplace/pkg/apis/marketplace/v1alpha1"
	"github.com/operator-framework/operator-marketplace/pkg/datastore"
	"github.com/sirupsen/logrus"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	containerName   = "registry-server"
	clusterRoleName = "marketplace-operator-registry-server"
	portNumber      = 50051
	portName        = "grpc"
)

var (
	action = []string{"grpc_health_probe", "-addr=localhost:50051"}
)

type catalogSourceConfigWrapper struct {
	*v1alpha1.CatalogSourceConfig
}

func (c *catalogSourceConfigWrapper) key() client.ObjectKey {
	return client.ObjectKey{
		Name:      c.GetName(),
		Namespace: c.GetTargetNamespace(),
	}
}

type registry struct {
	log      *logrus.Entry
	client   client.Client
	reader   datastore.Reader
	csc      catalogSourceConfigWrapper
	image    string
	address  string
	checksum string
}

// Registry contains the method that ensures a registry-pod deployment and its
// associated resources are created.
type Registry interface {
	Ensure() error
	GetAddress() string
}

// NewRegistry returns an initialized instance of Registry
func NewRegistry(log *logrus.Entry, client client.Client, reader datastore.Reader, csc *v1alpha1.CatalogSourceConfig, image string) Registry {
	return &registry{
		log:    log,
		client: client,
		reader: reader,
		csc:    catalogSourceConfigWrapper{csc},
		image:  image,
	}
}

// Ensure ensures a registry-pod deployment and its associated
// resources are created.
func (r *registry) Ensure() error {
	if err := r.ensureServiceAccount(); err != nil {
		return err
	}
	if err := r.ensureClusterRole(); err != nil {
		return err
	}
	if err := r.ensureClusterRoleBinding(); err != nil {
		return err
	}
	if err := r.ensureDeployment(); err != nil {
		return err
	}
	if err := r.ensureService(); err != nil {
		return err
	}
	return nil
}

func (r *registry) GetAddress() string {
	return r.address
}

// ensureClusterRole ensure that the ClusterRole required to be associated with
// the Deployment is present. We reuse the same ClusterRole across all registry
// deployments
func (r *registry) ensureClusterRole() error {
	clusterRole := new(ClusterRoleBuilder).WithTypeMeta().ClusterRole()
	if err := r.client.Get(context.TODO(), client.ObjectKey{Name: clusterRoleName}, clusterRole); err != nil {
		clusterRole = r.newClusterRole()
		err = r.client.Create(context.TODO(), clusterRole)
		if err != nil && !errors.IsAlreadyExists(err) {
			r.log.Errorf("Failed to create ClusterRole %s: %v", clusterRole.GetName(), err)
			return err
		}
		r.log.Infof("Created ClusterRole %s", clusterRole.GetName())
	} else {
		// Update the Rules to be on the safe side
		clusterRole.Rules = r.getClusterRules()
		err = r.client.Update(context.TODO(), clusterRole)
		if err != nil {
			r.log.Errorf("Failed to update ClusterRole %s : %v", clusterRole.GetName(), err)
			return err
		}
		r.log.Infof("Updated ClusterRole %s", clusterRole.GetName())
	}
	return nil
}

// ensureClusterRoleBinding ensures that the ClusterRoleBinding bound to the
// ClusterRole previously created is present.
func (r *registry) ensureClusterRoleBinding() error {
	clusterRoleBinding := new(ClusterRoleBindingBuilder).WithTypeMeta().ClusterRoleBinding()
	if err := r.client.Get(context.TODO(), r.csc.key(), clusterRoleBinding); err != nil {
		clusterRoleBinding = r.newClusterRoleBinding()
		err = r.client.Create(context.TODO(), clusterRoleBinding)
		if err != nil && !errors.IsAlreadyExists(err) {
			r.log.Errorf("Failed to create ClusterRoleBinding %s: %v", clusterRoleBinding.GetName(), err)
			return err
		}
		r.log.Infof("Created ClusterRoleBinding %s", clusterRoleBinding.GetName())
	} else {
		// Update the Rules to be on the safe side
		clusterRoleBinding.RoleRef = NewClusterRoleRef(clusterRoleName)
		err = r.client.Update(context.TODO(), clusterRoleBinding)
		if err != nil {
			r.log.Errorf("Failed to update ClusterRoleBinding %s : %v", clusterRoleBinding.GetName(), err)
			return err
		}
		r.log.Infof("Updated ClusterRoleBinding %s", clusterRoleBinding.GetName())
	}
	return nil
}

func (r *registry) ensureDeployment() error {
	deployment := new(DeploymentBuilder).WithTypeMeta().Deployment()
	if err := r.client.Get(context.TODO(), r.csc.key(), deployment); err != nil {
		deployment = r.newDeployment()
		err = r.client.Create(context.TODO(), deployment)
		if err != nil && !errors.IsAlreadyExists(err) {
			r.log.Errorf("Failed to create Deployment %s: %v", deployment.GetName(), err)
			return err
		}
		r.log.Infof("Created Deployment %s", deployment.GetName())
	} else {
		// Update the pod specification
		deployment.Spec.Template = r.newPodTemplateSpec()
		err = r.client.Update(context.TODO(), deployment)
		if err != nil {
			r.log.Errorf("Failed to update Deployment %s : %v", deployment.GetName(), err)
			return err
		}
		r.log.Infof("Updated Deployment %s", deployment.GetName())
	}
	return r.waitForDeployment(deployment, 5*time.Second, 5*time.Minute)
}

// ensureRole ensure that the Role required to be associated with the
// Deployment is present.
func (r *registry) ensureRole() error {
	role := new(RoleBuilder).WithTypeMeta().Role()
	if err := r.client.Get(context.TODO(), r.csc.key(), role); err != nil {
		role = r.newRole()
		err = r.client.Create(context.TODO(), role)
		if err != nil && !errors.IsAlreadyExists(err) {
			r.log.Errorf("Failed to create Role %s: %v", role.GetName(), err)
			return err
		}
		r.log.Infof("Created Role %s", role.GetName())
	} else {
		// Update the Rules to be on the safe side
		role.Rules = r.getRules()
		err = r.client.Update(context.TODO(), role)
		if err != nil {
			r.log.Errorf("Failed to update Role %s : %v", role.GetName(), err)
			return err
		}
		r.log.Infof("Updated Role %s", role.GetName())
	}
	return nil
}

// ensureRoleBinding ensures that the RoleBinding bound to the Role previously
// created is present.
func (r *registry) ensureRoleBinding() error {
	roleBinding := new(RoleBindingBuilder).WithTypeMeta().RoleBinding()
	if err := r.client.Get(context.TODO(), r.csc.key(), roleBinding); err != nil {
		roleBinding = r.newRoleBinding(r.csc.GetName())
		err = r.client.Create(context.TODO(), roleBinding)
		if err != nil && !errors.IsAlreadyExists(err) {
			r.log.Errorf("Failed to create RoleBinding %s: %v", roleBinding.GetName(), err)
			return err
		}
		r.log.Infof("Created RoleBinding %s", roleBinding.GetName())
	} else {
		// Update the Rules to be on the safe side
		roleBinding.RoleRef = NewRoleRef(r.csc.GetName())
		err = r.client.Update(context.TODO(), roleBinding)
		if err != nil {
			r.log.Errorf("Failed to update RoleBinding %s : %v", roleBinding.GetName(), err)
			return err
		}
		r.log.Infof("Updated RoleBinding %s", roleBinding.GetName())
	}
	return nil
}

// ensureService ensure that the Service for the registry deployment is present.
func (r *registry) ensureService() error {
	service := new(ServiceBuilder).WithTypeMeta().Service()
	if err := r.client.Get(context.TODO(), r.csc.key(), service); err != nil {
		service = r.newService()
		err = r.client.Create(context.TODO(), service)
		if err != nil && !errors.IsAlreadyExists(err) {
			r.log.Errorf("Failed to create Service %s: %v", service.GetName(), err)
			return err
		}
		r.log.Infof("Created Service %s", service.GetName())
	} else {
		r.log.Infof("Service %s is present", service.GetName())
	}

	r.address = service.GetName() + "." + service.GetNamespace() +
		".svc.cluster.local:" + strconv.Itoa(int(service.Spec.Ports[0].Port))
	return nil
}

// ensureServiceAccount ensure that the ServiceAccount required to be associated
// with the Deployment is present.
func (r *registry) ensureServiceAccount() error {
	serviceAccount := new(ServiceAccountBuilder).WithTypeMeta().ServiceAccount()
	if err := r.client.Get(context.TODO(), r.csc.key(), serviceAccount); err != nil {
		serviceAccount = r.newServiceAccount()
		err = r.client.Create(context.TODO(), serviceAccount)
		if err != nil && !errors.IsAlreadyExists(err) {
			r.log.Errorf("Failed to create ServiceAccount %s: %v", serviceAccount.GetName(), err)
			return err
		}
		r.log.Infof("Created ServiceAccount %s", serviceAccount.GetName())
	} else {
		r.log.Infof("ServiceAccount %s is present", serviceAccount.GetName())
	}
	return nil
}

// getCommand returns the command used to launch the registry server
func (r *registry) getCommand(packages string, sources string) []string {
	return []string{"appregistry-server", "debug", "-t", "termination.log", "-s", sources, "-o", packages}
}

// getLabels returns the label that must match between the Deployment's
// LabelSelector and the Pod template's label
func (r *registry) getLabel() map[string]string {
	return map[string]string{"marketplace.catalogSourceConfig": r.csc.GetName()}
}

// getPackagesAndSources returns the packageIDs and namespaced OperatorSources
// that can be used to construct the command line for the registry pod
func (r *registry) getPackagesAndSources() (string, string) {
	var opsrcList string
	for _, packageID := range GetPackageIDs(r.csc.Spec.Packages) {
		opsrcMeta, err := r.reader.Read(packageID)
		if err != nil {
			r.log.Errorf("Error %v reading package %s", err, packageID)
			continue
		}
		opsrcNamespacedName := opsrcMeta.Namespace + "/" + opsrcMeta.Name
		if !strings.Contains(opsrcList, opsrcNamespacedName) {
			opsrcList += opsrcNamespacedName + ","
		}
	}
	opsrcList = strings.TrimSuffix(opsrcList, ",")
	return r.csc.Spec.Packages, opsrcList
}

// getRules returns the PolicyRule needed to access the required resources from
// the registry pod
func (r *registry) getRules() []rbac.PolicyRule {
	return []rbac.PolicyRule{
		NewRule([]string{"get"}, []string{"marketplace.redhat.com"}, []string{"operatorsources"}, []string{r.csc.GetName()}),
	}
}

// getClusterRules returns the PolicyRule needed to access the required resources from
// the registry pod
func (r *registry) getClusterRules() []rbac.PolicyRule {
	return []rbac.PolicyRule{
		NewClusterRule([]string{"get"}, []string{"marketplace.redhat.com"}, []string{"operatorsources"}),
	}
}

// getSubjects returns the Subjects that the RoleBinding should apply to.
func (r *registry) getSubjects() []rbac.Subject {
	return []rbac.Subject{
		{
			Kind:      "ServiceAccount",
			Name:      r.csc.GetName(),
			Namespace: r.csc.GetTargetNamespace(),
		},
	}
}

// newDeployment() returns a Deployment object that can be used to bring up a
// registry deployment
func (r *registry) newDeployment() *apps.Deployment {
	return new(DeploymentBuilder).
		WithMeta(r.csc.GetName(), r.csc.GetTargetNamespace()).
		WithOwner(r.csc.CatalogSourceConfig).
		WithSpec(1, r.getLabel(), r.newPodTemplateSpec()).
		Deployment()
}

// newPodTemplateSpec returns a PodTemplateSpec object that can be used to bring up a registry pod
func (r *registry) newPodTemplateSpec() core.PodTemplateSpec {
	return core.PodTemplateSpec{
		ObjectMeta: meta.ObjectMeta{
			Name:      r.csc.GetName(),
			Namespace: r.csc.GetTargetNamespace(),
			Labels:    r.getLabel(),
		},
		Spec: core.PodSpec{
			Containers: []core.Container{
				{
					Name:    r.csc.GetName(),
					Image:   r.image,
					Command: r.getCommand(r.getPackagesAndSources()),
					Ports: []core.ContainerPort{
						{
							Name:          portName,
							ContainerPort: portNumber,
						},
					},
					ReadinessProbe: &core.Probe{
						Handler: core.Handler{
							Exec: &core.ExecAction{
								Command: action,
							},
						},
						InitialDelaySeconds: 1,
					},
					LivenessProbe: &core.Probe{
						Handler: core.Handler{
							Exec: &core.ExecAction{
								Command: action,
							},
						},
						InitialDelaySeconds: 2,
					},
				},
			},
			ServiceAccountName: r.csc.GetName(),
		},
	}
}

// newClusterRole returns a ClusterRole object with the rules set to access the
// required resources from the registry pod
func (r *registry) newClusterRole() *rbac.ClusterRole {
	return new(ClusterRoleBuilder).
		WithMeta(clusterRoleName).
		WithRules(r.getRules()).
		ClusterRole()
}

// newClusterRoleBinding returns a ClusterRoleBinding object RoleRef set to the given Role.
func (r *registry) newClusterRoleBinding() *rbac.ClusterRoleBinding {
	return new(ClusterRoleBindingBuilder).
		WithMeta(r.csc.GetName()).
		WithOwner(r.csc.CatalogSourceConfig).
		WithSubjects(r.getSubjects()).
		WithClusterRoleRef(clusterRoleName).
		ClusterRoleBinding()
}

// newRole returns a Role object with the rules set to access the required
// resources from the registry pod
func (r *registry) newRole() *rbac.Role {
	return new(RoleBuilder).
		WithMeta(r.csc.GetName(), r.csc.GetNamespace()).
		WithOwner(r.csc.CatalogSourceConfig).
		WithRules(r.getRules()).
		Role()
}

// newRoleBinding returns a RoleBinding object RoleRef set to the given Role.
func (r *registry) newRoleBinding(roleName string) *rbac.RoleBinding {
	return new(RoleBindingBuilder).
		WithMeta(r.csc.GetName(), r.csc.GetTargetNamespace()).
		WithOwner(r.csc.CatalogSourceConfig).
		WithSubjects(r.getSubjects()).
		WithRoleRef(roleName).
		RoleBinding()
}

// newService returns a new Service object.
func (r *registry) newService() *core.Service {
	return new(ServiceBuilder).
		WithMeta(r.csc.GetName(), r.csc.GetTargetNamespace()).
		WithOwner(r.csc.CatalogSourceConfig).
		WithSpec(r.newServiceSpec()).
		Service()
}

// newServiceAccount returns a new ServiceAccount object.
func (r *registry) newServiceAccount() *core.ServiceAccount {
	return new(ServiceAccountBuilder).
		WithMeta(r.csc.GetName(), r.csc.GetTargetNamespace()).
		WithOwner(r.csc.CatalogSourceConfig).
		ServiceAccount()
}

// newServiceSpec returns a ServiceSpec as required to front the registry deployment
func (r *registry) newServiceSpec() core.ServiceSpec {
	return core.ServiceSpec{
		Ports: []core.ServicePort{
			{
				Name:       portName,
				Port:       portNumber,
				TargetPort: intstr.FromInt(portNumber),
			},
		},
		Selector: r.getLabel(),
	}
}

func (r *registry) waitForDeployment(deployment *apps.Deployment, retryInterval, timeout time.Duration) error {
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		err = r.client.Get(context.TODO(), r.csc.key(), deployment)
		if err != nil {
			if errors.IsNotFound(err) {
				r.log.Infof("Waiting for availability of %s deployment", deployment.GetName())
				return false, nil
			}
			return false, err
		}

		if int32(deployment.Status.AvailableReplicas) == int32(*deployment.Spec.Replicas) {
			return true, nil
		}
		r.log.Infof("Waiting for full availability of %s deployment (%d/%d)", deployment.GetName(),
			deployment.Status.AvailableReplicas, *deployment.Spec.Replicas)
		return false, nil
	})
	if err != nil {
		return err
	}
	r.log.Infof("Deployment available (%d/%d)", deployment.Status.AvailableReplicas, *deployment.Spec.Replicas)
	return nil
}
