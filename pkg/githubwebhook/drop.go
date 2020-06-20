package githubwebhook

import (
	"context"
	"fmt"

	deployv1alpha1 "github.com/michaelbeaumont/properator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type drop struct {
	pr prPointer
}

func (d *drop) Act(webhook *WebhookHandler) error {
	ctx := context.Background()
	name, namespace := d.pr.getNamespaced()
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
