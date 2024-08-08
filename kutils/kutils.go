package kutils

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type KubeClient interface {
	Get(ctx context.Context, key client.ObjectKey, obj client.Object) error
	List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error
	Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error
	Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error
	Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error
	Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error
	DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error
}

type ObjectState string

const (
	ObjectCreated  ObjectState = "CREATED"
	ObjectUpdated  ObjectState = "UPDATED"
	ObjectPatched  ObjectState = "PATCHED"
	ObjectDeleted  ObjectState = "DELETED"
	ObjectNotFound ObjectState = "NOT_FOUND"

//    ObjectNotModified ObjectState = "NOT_MODIFIED"
)

func ExtractObjectKey(obj client.Object) client.ObjectKey {
	return client.ObjectKey{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
}

// KubeApply is the equivalent of kubectl apply functionality
// It takes any kubernetes object, checks whether it exists or not,
// and creates/updates it accordingly
func KubeApply(c KubeClient, ctx context.Context, obj client.Object) (ObjectState, error) {
	err := c.Get(ctx, ExtractObjectKey(obj), obj)

	if err != nil {
		if errors.IsNotFound(err) {
			err = c.Create(ctx, obj)
			if err != nil {
				return "", err
			}
			return ObjectCreated, nil
		}
		return "", err
	}

	err = c.Update(ctx, obj)
	if err != nil {
		return "", err
	}
	return ObjectUpdated, nil
}

// KubeDestroy destroys the given kubernetes object
// If the object exists, it deletes it, else does not take any action
func KubeDestroy(c KubeClient, ctx context.Context, obj client.Object) (ObjectState, error) {
	err := c.Get(ctx, ExtractObjectKey(obj), obj)
	if err != nil {
		if errors.IsNotFound(err) {
			return ObjectNotFound, nil
		}
		return "", err
	}

	err = c.Delete(ctx, obj)
	if err != nil {
		return "", err
	}
	return ObjectDeleted, nil
}
