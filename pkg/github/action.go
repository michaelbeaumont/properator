package github

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	deployv1alpha1 "github.com/michaelbeaumont/properator/api/v1alpha1"
	"github.com/michaelbeaumont/properator/pkg/utils"
)

type action interface {
	Act(webhook *WebhookHandler) error
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

func (ca *noopAction) Act(webhook *WebhookHandler) error {
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

func (ca *create) Act(webhook *WebhookHandler) error {
	ctx := context.Background()
	pr, _, err := webhook.ghCli.PullRequests.Get(ctx, ca.owner, ca.name, ca.pr.number)
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
		},
	}
	ghDeployment := deployv1alpha1.GithubDeployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: deployv1alpha1.Deployment{
			Owner: ca.owner,
			Name:  ca.name,
			Ref:   ref,
		},
	}
	if err := utils.CreateOrReplace(ctx, webhook.k8s, webhook.k8s, &refRelease); err != nil {
		return err
	}
	ns, _ := client.ObjectKeyFromObject(&ghDeployment)
	if err := webhook.k8s.Get(ctx, ns, &deployv1alpha1.GithubDeployment{}); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return err
		}
		if err := webhook.k8s.Create(ctx, &ghDeployment); err != nil {
			return err
		}
	}

	return nil
}
func (ca *create) Describe() string {
	return fmt.Sprintf("Creating PR %d from %d", ca.pr.number, ca.pr.id)
}

func (d *drop) Act(webhook *WebhookHandler) error {
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
	return nil
}
func (d *drop) Describe() string {
	return fmt.Sprintf("Dropping PR %d from %d", d.pr.number, d.pr.id)
}
