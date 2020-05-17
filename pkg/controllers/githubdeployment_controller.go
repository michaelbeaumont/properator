package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	gh "github.com/google/go-github/v31/github"
	deployv1alpha1 "github.com/michaelbeaumont/properator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:rbac:groups=deploy.properator.io,resources=githubdeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=deploy.properator.io,resources=githubdeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=secrets;configmaps;serviceaccounts,verbs=get;create;update

// GithubDeploymentReconciler reconciles a GithubDeployment object.
type GithubDeploymentReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	GhCli  *gh.Client
}

var (
	transientEnvironment = true
	autoMerge            = false
	baseEnvironment      = "properator"
	inactive             = "inactive"
	inactiveStatus       = gh.DeploymentStatusRequest{
		State: &inactive,
	}
)

func createStatus(
	ctx context.Context, ghCli *gh.Client, gd *deployv1alpha1.GithubDeployment, status *gh.DeploymentStatusRequest,
) error {
	_, _, err := ghCli.Repositories.CreateDeploymentStatus(
		ctx, gd.Spec.Owner, gd.Spec.Name, gd.Spec.ID, status,
	)

	return err
}

//ReconcileStatus handles telling Github about the status.
func ReconcileStatus(
	ctx context.Context, ghCli *gh.Client, gd *deployv1alpha1.GithubDeployment,
) (bool, error) {
	st := &gd.Status
	sp := &gd.Spec

	if sp.Status != *st {
		*st = sp.Status
		status := gh.DeploymentStatusRequest{
			State: &st.State,
		}

		if st.URL != "" {
			status.EnvironmentURL = &st.URL
		}

		// TODO retry on certain GH errors?
		if *status.State != "" {
			return true, createStatus(ctx, ghCli, gd, &status)
		}

		return true, nil
	}

	return false, nil
}

func (r *GithubDeploymentReconciler) createDeployment(
	ctx context.Context, gd *deployv1alpha1.GithubDeployment,
) (*gh.Deployment, error) {
	environment := fmt.Sprintf("%s (%s)", baseEnvironment, gd.Spec.Ref)
	depReq := gh.DeploymentRequest{
		Ref:                  &gd.Spec.Ref,
		Environment:          &environment,
		AutoMerge:            &autoMerge,
		TransientEnvironment: &transientEnvironment,
	}
	dep, _, err := r.GhCli.Repositories.CreateDeployment(ctx, gd.Spec.Owner, gd.Spec.Name, &depReq)

	if err != nil {
		return nil, err
	}

	return dep, nil
}

const deactivateFinalizer string = "finalizers.deploy.properator.io/deactivate"

// ensureFinalizer makes sure our finalizer is present
// it returns whether anything was changed.
func ensureFinalizer(gd *deployv1alpha1.GithubDeployment) bool {
	needsFinalizer := true
	for _, item := range gd.Finalizers {
		needsFinalizer = needsFinalizer && item != deactivateFinalizer
	}

	if needsFinalizer {
		gd.Finalizers = append(gd.Finalizers, deactivateFinalizer)
		return true
	}

	return false
}

// dropFinalizer makes sure our finalizer is not present
// it returns whether anything was changed.
func dropFinalizer(gd *deployv1alpha1.GithubDeployment) bool {
	var remainingFinalizers []string

	for _, item := range gd.Finalizers {
		if item != deactivateFinalizer {
			remainingFinalizers = append(remainingFinalizers, item)
		}
	}

	if len(remainingFinalizers) < len(gd.Finalizers) {
		gd.Finalizers = remainingFinalizers
		return true
	}

	return false
}

func (r *GithubDeploymentReconciler) handleBeingDeleted(
	ctx context.Context, gd *deployv1alpha1.GithubDeployment,
) (ctrl.Result, error) {
	if dropFinalizer(gd) {
		status := gh.DeploymentStatusRequest{
			State: &inactive,
		}
		if err := createStatus(ctx, r.GhCli, gd, &status); err != nil {
			return ctrl.Result{}, err
		}

		if err := r.Update(ctx, gd); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Stop reconciliation as the item is being deleted
	return ctrl.Result{}, nil
}

// Reconcile handles GithubDeployments.
func (r *GithubDeploymentReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("githubdeployment", req.NamespacedName)

	var gd deployv1alpha1.GithubDeployment
	if err := r.Get(ctx, req.NamespacedName, &gd); err != nil {
		log.Error(err, "unable to fetch github deployments")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var err error

	if gd.Spec.ID == 0 {
		var dep *gh.Deployment
		dep, err = r.createDeployment(ctx, &gd)
		// Now handle potentially updated status

		if err != nil {
			log.Error(err, "unable to create deployment on github")
		} else {
			// If flux has updated, we have a successful new dep
			// we always have an active one
			gd.Spec.ID = dep.GetID()
			gd.Spec.Sha = dep.GetSHA()
		}
	}

	var needsUpdate bool

	if !gd.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.handleBeingDeleted(ctx, &gd)
	}

	needsUpdate = ensureFinalizer(&gd)

	statusUpdated, err := ReconcileStatus(ctx, r.GhCli, &gd)
	if err != nil {
		log.Error(err, "unable to update on github")
	}

	needsUpdate = needsUpdate || statusUpdated

	if needsUpdate {
		if err := r.Update(ctx, &gd); err != nil {
			log.Error(err, "unable to update github deployment resource")
		}
	}

	return ctrl.Result{}, err
}

// SetupWithManager initializes our controller.
func (r *GithubDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&deployv1alpha1.GithubDeployment{}).
		Complete(r)
}
