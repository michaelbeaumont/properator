package githubwebhook

import (
	"context"
	"fmt"
	"net/http"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gh "github.com/google/go-github/v31/github"
	deployv1alpha1 "github.com/michaelbeaumont/properator/api/v1alpha1"
	"github.com/michaelbeaumont/properator/pkg/utils"
	"github.com/pkg/errors"
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
	readOnlyKey          = true
	autoMerge            = false
	properator           = "properator"
	success              = "success"
	inactive             = "inactive"
)

const annotation = "deploy.properator.io/github-webhook"

func (ca *create) ensureGitKeySecret(ctx context.Context, webhook *WebhookHandler) (secretName string, err error) {
	name := fmt.Sprintf("properator-git-deploy-key-%v", ca.pr.id)
	currentNs, err := utils.GetCurrentNamespace()
	if err != nil {
		return "", err
	}
	secretNN := types.NamespacedName{Name: name, Namespace: currentNs}
	if err := webhook.k8s.Get(ctx, secretNN, &v1.Secret{}); err != nil {
		keyPair, err := GenerateKey()
		if err != nil {
			return "", err
		}
		keySecret := v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: name, Namespace: currentNs,
			},
			Data: map[string][]byte{
				"identity": keyPair.Private,
			},
			Type: v1.SecretTypeOpaque,
		}
		if err := webhook.k8s.Create(ctx, &keySecret); err != nil {
			return "", err
		}
		pubKey := string(keyPair.Public)
		ghKey := gh.Key{Title: &properator, Key: &pubKey, ReadOnly: &readOnlyKey}
		_, resp, err := webhook.ghCli.Repositories.CreateKey(ctx, ca.owner, ca.name, &ghKey)
		if err != nil {
			return "", nil
		}
		if resp.StatusCode != http.StatusCreated {
			return "", errors.Errorf("Unable to create deploy key for repository %s/%s", ca.owner, ca.name)
		}
	}
	return name, nil
}

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
	keySecretName, err := ca.ensureGitKeySecret(ctx, webhook)
	if err != nil {
		return err
	}
	refRelease := deployv1alpha1.RefRelease{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: deployv1alpha1.RefReleaseSpec{
			Repo: deployv1alpha1.Repo{
				Owner:         ca.owner,
				Name:          ca.name,
				KeySecretName: keySecretName,
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
