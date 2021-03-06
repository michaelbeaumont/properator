package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
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
	APIReader client.Reader
}

// Reconcile handles RefRelease
func (r *RefReleaseReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("refrelease", req.NamespacedName)
	var refRelease deployv1alpha1.RefRelease
	if err := r.Get(ctx, req.NamespacedName, &refRelease); err != nil {
		return ctrl.Result{}, errors.Wrap(client.IgnoreNotFound(err), "unable to fetch release")
	}

	flux, err := FluxResources(ctx, r.APIReader, refRelease.ObjectMeta, refRelease.Spec)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "unable to generate flux resources")
	}
	if err := flux.GiveOwnership(&refRelease, r.Scheme); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "unable to take ownership of flux")
	}
	if err := flux.Deploy(ctx, log, r, r.APIReader); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "unable to deploy flux")
	}

	return ctrl.Result{}, nil
}

// SetupWithManager initializes our controller
func (r *RefReleaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&deployv1alpha1.RefRelease{}).
		Complete(r)
}
