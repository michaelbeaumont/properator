package controllers

import (
	"context"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	deployv1alpha1 "github.com/michaelbeaumont/properator/api/v1alpha1"
)

// +kubebuilder:rbac:groups=deploy.properator.io,resources=githubdeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

// SecretReconciler reconciles secrets
// GitKey should be base64 encoded
type SecretReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// Reconcile handles Secrets
func (sr *SecretReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := sr.Log.WithValues("secret", req.NamespacedName)

	var secret v1.Secret
	if err := sr.Get(ctx, req.NamespacedName, &secret); err != nil {
		log.Error(err, "unable to fetch secret")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	fluxAnnotation, ok := secret.Annotations["flux.weave.works/sync-hwm"]

	if !ok || fluxAnnotation == "" {
		return ctrl.Result{}, nil
	}

	deploymentName := "github-webhook"
	ns := client.ObjectKey{
		Name:      deploymentName,
		Namespace: secret.Namespace,
	}

	var gd deployv1alpha1.GithubDeployment
	if err := sr.Get(ctx, ns, &gd); err != nil {
		log.Info("unable to get githubdeployment from annotation", "deployment", deploymentName)
		return ctrl.Result{}, nil
	}

	if fluxAnnotation != gd.Spec.Sha {
		log.Info("Updating github deployment")

		gd.Spec.Sha = fluxAnnotation
		if err := sr.Update(ctx, &gd); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager initializes our controller
func (sr *SecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Secret{}).
		Complete(sr)
}
