package controllers

import (
	"context"

	"github.com/go-logr/logr"
	networkv1 "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	deployv1alpha1 "github.com/michaelbeaumont/properator/api/v1alpha1"
)

// +kubebuilder:rbac:groups=deploy.properator.io,resources=githubdeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch

// IngressReconciler reconciles an Ingress object
// GitKey should be base64 encoded.
type IngressReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// Reconcile handles Ingresses.
func (ir *IngressReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := ir.Log.WithValues("ingress", req.NamespacedName)

	var ingress networkv1.Ingress
	if err := ir.Get(ctx, req.NamespacedName, &ingress); err != nil {
		log.Error(err, "unable to fetch ingress")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	deploymentName, okDep := ingress.Annotations["deploy.properator.io/deployment"]

	if !okDep {
		return ctrl.Result{}, nil
	}

	ns := client.ObjectKey{
		Name:      deploymentName,
		Namespace: ingress.Namespace,
	}

	var gd deployv1alpha1.GithubDeployment
	if err := ir.Get(ctx, ns, &gd); err != nil {
		log.Info("unable to get githubdeployment from annotation", "deployment", deploymentName)
		return ctrl.Result{}, nil
	}

	status := deployv1alpha1.DeploymentStatus{
		State: "success",
	}
	URL, ok := ingress.Annotations["deploy.properator.io/url"]

	if ok {
		status.URL = URL
	}

	gd.Spec.Status = status

	if err := ir.Update(ctx, &gd); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager initializes our controller.
func (ir *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkv1.Ingress{}).
		Complete(ir)
}
