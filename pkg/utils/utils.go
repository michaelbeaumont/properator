package utils

import (
	"context"
	"io/ioutil"
	"reflect"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateOrReplace simply creates or overwrites a resource.
func CreateOrReplace(ctx context.Context, r client.Reader, c client.Client, obj runtime.Object) error {
	ns, _ := client.ObjectKeyFromObject(obj)
	// Hacky hacky but I don't want to DeepCopy `obj`
	// and passing nil doesn't supply `Get` with the type
	typ := reflect.TypeOf(obj).Elem()
	ignored := reflect.New(typ).Interface().(runtime.Object)

	if err := r.Get(ctx, ns, ignored); err != nil {
		if err := c.Create(ctx, obj); err != nil {
			return errors.Wrapf(err, "couldn't create %s", typ.Name())
		}
	} else if err := c.Update(ctx, obj); err != nil {
		return errors.Wrapf(err, "couldn't update %s", typ.Name())
	}

	return nil
}

// GetCurrentNamespace gives us the namespace of the running pod.
func GetCurrentNamespace() (string, error) {
	data, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "", errors.Wrap(err, "couldn't read namespace from server account")
	}

	return string(data), nil
}
