package controllers

import (
	"context"
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	deployv1alpha1 "github.com/michaelbeaumont/properator/api/v1alpha1"
	"github.com/michaelbeaumont/properator/pkg/utils"
)

const (
	fluxDeployKeyName = "properator-git-deploy-key"
)

// Flux holds all k8s resources needed for flux.
type Flux struct {
	deployment     appsv1.Deployment
	configMap      v1.ConfigMap
	secret         v1.Secret
	serviceAccount v1.ServiceAccount
	roleBinding    rbacv1.RoleBinding
}

type object interface {
	runtime.Object
	metav1.Object
}

func (f *Flux) toObjectList() []object {
	return []object{&f.deployment, &f.configMap, &f.secret, &f.serviceAccount, &f.roleBinding}
}

// GiveOwnership sets controller references for Flux resources.
func (f *Flux) GiveOwnership(owner metav1.Object, scheme *runtime.Scheme) error {
	for _, obj := range f.toObjectList() {
		if err := ctrl.SetControllerReference(owner, obj, scheme); err != nil {
			return err
		}
	}

	return nil
}

// Deploy deploys this Flux instance to the cluster.
func (f *Flux) Deploy(ctx context.Context, log logr.Logger, c client.Client, r client.Reader) error {
	for _, obj := range f.toObjectList() {
		if err := utils.CreateOrReplace(ctx, r, c, obj); err != nil {
			return err
		}
	}

	return nil
}

// Resource creation

// FluxResources creates the k8s resources needed to launch flux.
func FluxResources(
	ctx context.Context, r client.Reader, meta metav1.ObjectMeta, spec deployv1alpha1.RefReleaseSpec,
) (Flux, error) {
	repo := spec.Repo
	ref := spec.Ref
	fullName := fmt.Sprintf("%s/%s", repo.Owner, repo.Name)
	repoURL := fmt.Sprintf("git@github.com:%[1]s", fullName)

	var refStr string
	if ref.Branch != "" {
		refStr = ref.Branch
	} else {
		refStr = ref.Tag
	}

	data := map[string]string{
		"ref": refStr,
		"sha": ref.Sha,
	}
	if ref.PullRequest != 0 {
		data["pr"] = strconv.Itoa(ref.PullRequest)
	}

	configMap := v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      meta.Name,
			Namespace: meta.Namespace,
		},
		Data: data,
	}
	deployment := fluxDeployment(meta, repoURL, ref.Branch)
	sa, rb := fluxRbac(meta)

	secret, err := fluxSecret(ctx, r, repo.KeySecretName, meta.Namespace)
	if err != nil {
		return Flux{}, errors.Wrap(err, "couldn't create flux secret")
	}

	return Flux{
		deployment,
		configMap,
		secret,
		sa,
		rb,
	}, nil
}

func fluxSecret(
	ctx context.Context, r client.Reader, keySecretName string, refNamespace string,
) (v1.Secret, error) {
	namespace, err := utils.GetCurrentNamespace()
	if err != nil {
		return v1.Secret{}, err
	}

	nn := types.NamespacedName{Name: keySecretName, Namespace: namespace}

	var commonSecret v1.Secret
	if err := r.Get(ctx, nn, &commonSecret); err != nil {
		return v1.Secret{}, err
	}

	return v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: fluxDeployKeyName, Namespace: refNamespace,
		},
		Data: commonSecret.Data,
		Type: commonSecret.Type,
	}, nil
}

func fluxContainer(namespace, repo, ref string) v1.Container {
	var port, probeSeconds int32 = 3030, 5

	return v1.Container{
		Name:  "flux",
		Image: "docker.io/fluxcd/flux:1.19.0",
		Resources: v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceCPU:    resource.MustParse("50m"),
				v1.ResourceMemory: resource.MustParse("64Mi"),
			},
		},
		Ports: []v1.ContainerPort{
			{
				ContainerPort: port,
			},
		},
		LivenessProbe: &v1.Probe{
			Handler: v1.Handler{
				HTTPGet: &v1.HTTPGetAction{
					Port: intstr.FromInt(int(port)),
					Path: "/api/flux/v6/identity.pub",
				},
			},
			InitialDelaySeconds: probeSeconds,
			TimeoutSeconds:      probeSeconds,
		},
		ReadinessProbe: &v1.Probe{
			Handler: v1.Handler{
				HTTPGet: &v1.HTTPGetAction{
					Port: intstr.FromInt(int(port)),
					Path: "/api/flux/v6/identity.pub",
				},
			},
			InitialDelaySeconds: probeSeconds,
			TimeoutSeconds:      probeSeconds,
		},
		VolumeMounts: []v1.VolumeMount{
			{
				Name:      "git-key",
				MountPath: "/etc/fluxd/ssh",
			},
			{
				Name:      "properator",
				MountPath: "/etc/properator",
			},
		},
		Args: []string{
			fmt.Sprintf("--git-url=%s", repo),
			fmt.Sprintf("--git-branch=%s", ref),
			//# - --git-path=subdir1,subdir2
			"--git-label=flux",
			"--git-readonly",
			"--sync-garbage-collection",
			fmt.Sprintf("--k8s-secret-name=%s", fluxDeployKeyName),
			"--registry-disable-scanning",
			fmt.Sprintf("--k8s-default-namespace=%s", namespace),
			"--manifest-generation=true",
		},
	}
}

func fluxDeployment(meta metav1.ObjectMeta, repo, ref string) appsv1.Deployment {
	var keyFileMode int32 = 0400

	return appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      meta.Name,
			Namespace: meta.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": meta.Name,
				},
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"name": meta.Name,
					},
					Annotations: map[string]string{
						"prometheus.io/port": "3031",
					},
				},
				Spec: v1.PodSpec{
					ServiceAccountName: meta.Name,
					Volumes: []v1.Volume{
						{
							Name: "git-key",
							VolumeSource: v1.VolumeSource{
								Secret: &v1.SecretVolumeSource{
									SecretName:  fluxDeployKeyName,
									DefaultMode: &keyFileMode,
								},
							},
						},
						{
							Name: "properator",
							VolumeSource: v1.VolumeSource{
								ConfigMap: &v1.ConfigMapVolumeSource{
									LocalObjectReference: v1.LocalObjectReference{
										Name: meta.Name,
									},
								},
							},
						},
					},
					Containers: []v1.Container{
						fluxContainer(meta.Namespace, repo, ref),
					},
				},
			},
		},
	}
}

func fluxRbac(meta metav1.ObjectMeta) (v1.ServiceAccount, rbacv1.RoleBinding) {
	sa := v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: meta.Name, Namespace: meta.Namespace},
	}
	rb := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: meta.Name, Namespace: meta.Namespace},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     "properator-flux",
			APIGroup: "rbac.authorization.k8s.io",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      meta.Name,
				Namespace: meta.Namespace,
			},
		},
	}

	return sa, rb
}
