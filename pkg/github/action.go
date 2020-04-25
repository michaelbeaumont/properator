package github

import (
	"context"
	"fmt"

	gh "github.com/google/go-github/v31/github"
	deployv1alpha1 "github.com/michaelbeaumont/properator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type action interface {
	Act(webhook WebhookWorker) error
	Describe() string
}

type prPointer struct {
	id     int64
	number int
}
type create struct {
	owner  string
	name   string
	branch string
	pr     prPointer
}
type drop struct {
	pr prPointer
}
type noopAction struct {
}

func (ca *noopAction) Act(webhook WebhookWorker) error {
	return nil
}

func (ca *noopAction) Describe() string {
	return ""
}

func getNamespaced(pr prPointer) (string, string) {
	return "github-webhook", fmt.Sprintf("properator-github-webhook-%v-%v", pr.id, pr.number)
}

var (
	transientEnvironment = true
	autoMerge            = false
	environment          = "properator"
	success              = "success"
	inactive             = "inactive"
)

const annotation = "deploy.properator.io/github-webhook"

func (ca *create) Act(webhook WebhookWorker) error {
	ctx := context.Background()
	pr, _, err := webhook.ghcli.PullRequests.Get(ctx, ca.owner, ca.name, ca.pr.number)
	if err != nil {
		return err
	}
	name, namespace := getNamespaced(ca.pr)

	ref := pr.GetHead().GetRef()
	nn := types.NamespacedName{Name: name, Namespace: namespace}
	if err := webhook.k8s.Get(ctx, nn, &v1.Namespace{}); err != nil {
		ns := v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace, Annotations: map[string]string{
					annotation: "true",
				},
			},
		}
		if err := webhook.k8s.Create(ctx, &ns); err != nil {
			return err
		}
	}
	var existing *deployv1alpha1.RefRelease
	var ghDeployment int64
	if err := webhook.k8s.Get(ctx, nn, existing); err == nil {
		ghDeployment = existing.Spec.GithubStatus.Deployment
	} else {
		depReq := gh.DeploymentRequest{
			Ref:                  &ref,
			Environment:          &environment,
			AutoMerge:            &autoMerge,
			TransientEnvironment: &transientEnvironment,
		}
		dep, _, err := webhook.ghcli.Repositories.CreateDeployment(ctx, ca.owner, ca.name, &depReq)
		if err != nil {
			return err
		}
		ghDeployment = dep.GetID()
	}
	refRelease := deployv1alpha1.RefRelease{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: deployv1alpha1.RefReleaseSpec{
			Repo: deployv1alpha1.Repo{
				Owner: ca.owner,
				Name:  ca.name,
			},
			Ref: deployv1alpha1.Ref{
				Sha:         pr.GetHead().GetSHA(),
				Branch:      ref,
				PullRequest: ca.pr.number,
			},
			GithubStatus: deployv1alpha1.GithubStatus{
				Deployment: ghDeployment,
			},
		},
	}
	ns, _ := client.ObjectKeyFromObject(&refRelease)
	if err := webhook.k8s.Get(ctx, ns, &deployv1alpha1.RefRelease{}); err != nil {
		if err := webhook.k8s.Create(ctx, &refRelease); err != nil {
			webhook.log.Info("created new refrelease")
			return err
		}
	} else {
		if err := webhook.k8s.Update(ctx, &refRelease); err != nil {
			webhook.log.Info("updated refrelease")
			return err
		}
	}

	status := gh.DeploymentStatusRequest{
		State: &success,
	}
	_, _, err = webhook.ghcli.Repositories.CreateDeploymentStatus(ctx, ca.owner, ca.name, ghDeployment, &status)
	return err
}
func (ca *create) Describe() string {
	return fmt.Sprintf("Creating PR %d from %d", ca.pr.number, ca.pr.id)
}

func (d *drop) Act(webhook WebhookWorker) error {
	ctx := context.Background()
	name, namespace := getNamespaced(d.pr)
	ns := v1.Namespace{}
	if err := webhook.k8s.Get(ctx, types.NamespacedName{Name: namespace}, &ns); err != nil {
		// Do nothing
		return nil
	}
	if _, ok := ns.Annotations[annotation]; !ok {
		// We don't own this ns apparently
		return nil
	}
	ref := deployv1alpha1.RefRelease{}
	if err := webhook.k8s.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &ref); err != nil {
		// Do nothing
		return nil
	}
	if err := webhook.k8s.Delete(ctx, &ns); err != nil {
		return err
	}
	status := gh.DeploymentStatusRequest{
		State: &inactive,
	}
	_, _, err := webhook.ghcli.Repositories.CreateDeploymentStatus(
		ctx, ref.Spec.Repo.Owner, ref.Spec.Repo.Name, ref.Spec.GithubStatus.Deployment, &status,
	)
	return err
}
func (d *drop) Describe() string {
	return fmt.Sprintf("Dropping PR %d from %d", d.pr.number, d.pr.id)
}
