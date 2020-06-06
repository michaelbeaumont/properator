package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	deployv1alpha1 "github.com/michaelbeaumont/properator/api/v1alpha1"
)

// +kubebuilder:rbac:groups=deploy.properator.io,resources=refreleases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=deploy.properator.io,resources=refreleases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;create;update
// +kubebuilder:rbac:groups=core,resources=secrets;configmaps;serviceaccounts,verbs=get;create;update
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;create;update
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=bind

// RefReleaseReconciler reconciles a RefRelease object
// GitKey should be base64 encoded
type RefReleaseReconciler struct {
	client.Client
	Log       logr.Logger
	Scheme    *runtime.Scheme
	GitKey    []byte
	APIReader client.Reader
}

// Reconcile handles RefRelease
func (r *RefReleaseReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("refrelease", req.NamespacedName)
	var refRelease deployv1alpha1.RefRelease
	if err := r.Get(ctx, req.NamespacedName, &refRelease); err != nil {
		log.Error(err, "unable to fetch release")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	repo := refRelease.Spec.Repo

	fullName := fmt.Sprintf("%s/%s", repo.Owner, repo.Name)
	repoURL := fmt.Sprintf("git@github.com:%[1]s", fullName)
	flux := FluxResources(refRelease.ObjectMeta, repoURL, refRelease.Spec.Ref, r.GitKey)
	if err := flux.GiveOwnership(&refRelease, r.Scheme); err != nil {
		log.Error(err, "unable to take ownership of flux")
	}
	if err := flux.Deploy(ctx, log, r, r.APIReader); err != nil {
		log.Error(err, "unable to deploy flux")
	}

	return ctrl.Result{}, nil
}

// SetupWithManager initializes our controller
func (r *RefReleaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&deployv1alpha1.RefRelease{}).
		Complete(r)
}
