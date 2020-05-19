package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	gh "github.com/google/go-github/v31/github"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	deployv1alpha1 "github.com/michaelbeaumont/properator/api/v1alpha1"
)

// +kubebuilder:rbac:groups=deploy.properator.io,resources=githubdeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=deploy.properator.io,resources=githubdeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=secrets;configmaps;serviceaccounts,verbs=get;create;update

// GithubDeploymentReconciler reconciles a GithubDeployment object
type GithubDeploymentReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	GhCli  *gh.Client
}

var (
	transientEnvironment = true
	autoMerge            = false
	autoInactive         = false
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

//ReconcileStatus handles telling Github about the status
func ReconcileStatus(
	ctx context.Context, ghCli *gh.Client, inactivateID int64, gd *deployv1alpha1.GithubDeployment,
) (bool, error) {
	st := &gd.Status
	sp := &gd.Spec

	if sp.Statuses[sp.Sha] != st.Status {
		st.Status = sp.Statuses[sp.Sha]
		status := gh.DeploymentStatusRequest{
			State: &st.Status.State,
		}

		if st.Status.URL != "" {
			status.EnvironmentURL = &st.Status.URL
		}

		if inactivateID != 0 {
			_, _, err := ghCli.Repositories.CreateDeploymentStatus(
				ctx, gd.Spec.Owner, gd.Spec.Name, inactivateID, &inactiveStatus,
			)
			if err != nil {
				return false, fmt.Errorf("couldn't deactivate old status: %w", err)
			}
		}
		// TODO retry on certain GH errors?
		return true, createStatus(ctx, ghCli, gd, &status)
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
// it returns whether anything was changed
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
// it returns whether anything was changed
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

// Reconcile handles GithubDeployments
func (r *GithubDeploymentReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("githubdeployment", req.NamespacedName)

	var gd deployv1alpha1.GithubDeployment
	if err := r.Get(ctx, req.NamespacedName, &gd); err != nil {
		log.Error(err, "unable to fetch github deployments")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var err error

	var inactivateID int64
	if gd.Spec.ID == 0 || gd.Spec.Sha != gd.Status.Sha {
		var dep *gh.Deployment
		dep, err = r.createDeployment(ctx, &gd)
		// Now handle potentially updated status, list of sha?

		if err != nil {
			log.Error(err, "unable to create deployment on github")
		} else {
			// Inactivate about the old ID
			inactivateID = gd.Spec.ID
			// If flux has updated, we have a successful new dep
			// we always have an active one
			gd.Spec.ID = dep.GetID()
			gd.Spec.Sha = dep.GetSHA()
			gd.Status.Sha = gd.Spec.Sha
		}
	}

	var needsUpdate bool

	if gd.ObjectMeta.DeletionTimestamp.IsZero() {
		needsUpdate = ensureFinalizer(&gd)
	} else {
		if dropFinalizer(&gd) {
			status := gh.DeploymentStatusRequest{
				State: &inactive,
			}
			if err := createStatus(ctx, r.GhCli, &gd, &status); err != nil {
				return ctrl.Result{}, err
			}

			if err := r.Update(ctx, &gd); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	statusUpdated, err := ReconcileStatus(ctx, r.GhCli, inactivateID, &gd)
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

// SetupWithManager initializes our controller
func (r *GithubDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&deployv1alpha1.GithubDeployment{}).
		Complete(r)
}
