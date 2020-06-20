package githubwebhook

import (
	"context"
	"fmt"
	"net/http"

	gh "github.com/google/go-github/v31/github"
	deployv1alpha1 "github.com/michaelbeaumont/properator/api/v1alpha1"
	"github.com/michaelbeaumont/properator/pkg/utils"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type create struct {
	owner  string
	name   string
	branch string
	pr     prPointer
}

func (ca *create) ensureGitKeySecret(ctx context.Context, webhook *WebhookHandler) (secretName string, err error) {
	name := fmt.Sprintf("properator-git-deploy-key-%v", ca.pr.id)
	currentNs, err := utils.GetCurrentNamespace()
	if err != nil {
		return "", err
	}
	secretNN := types.NamespacedName{Name: name, Namespace: currentNs}
	if err := webhook.k8s.Get(ctx, secretNN, &v1.Secret{}); err != nil {
		keyPair, err := generateKey()
		if err != nil {
			return "", errors.Wrap(err, "couldn't generate ssh key")
		}
		// save this key to our properator namespace
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
			return "", errors.Wrap(err, "couldn't create deploy key secret for repo")
		}
		// add this key to our repo's deploy keys
		pubKey := string(keyPair.Public)
		ghKey := gh.Key{Title: &properator, Key: &pubKey, ReadOnly: &readOnlyKey}
		_, resp, err := webhook.ghCli.Repositories.CreateKey(ctx, ca.owner, ca.name, &ghKey)
		if err != nil {
			return "", errors.Wrapf(err, "error creating deploy key for repository %s/%s", ca.owner, ca.name)
		}
		if resp.StatusCode != http.StatusCreated {
			return "", errors.Errorf("error creating deploy key for repository %s/%s: %d", ca.owner, ca.name, resp.StatusCode)
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
	name, namespace := ca.pr.getNamespaced()

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
		return errors.Wrap(err, "error ensuring git deploy key secret exists")
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
