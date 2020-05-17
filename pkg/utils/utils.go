package utils

import (
	"context"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateOrReplace simply creates or overwrites a resource
func CreateOrReplace(ctx context.Context, r client.Reader, c client.Client, obj runtime.Object) error {
	ns, _ := client.ObjectKeyFromObject(obj)
	// Hacky hacky but I don't want to DeepCopy `obj`
	// and passing nil doesn't supply `Get` with the type
	ignored := reflect.New(reflect.TypeOf(obj).Elem()).Interface().(runtime.Object)
	if err := r.Get(ctx, ns, ignored); err != nil {
		if err := c.Create(ctx, obj); err != nil {
			return err
		}
	} else if err := c.Update(ctx, obj); err != nil {
		return err
	}

	return nil
}
