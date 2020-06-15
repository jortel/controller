package ref

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

//
// Build an event handler.
// Example:
//   err = cnt.Watch(
//      &source.Kind{
//         Type: &api.Plan{},
//      },
//      libref.Handler())
func Handler() handler.EventHandler {
	return &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(
			func(a handler.MapObject) []reconcile.Request {
				return GetRequests(a)
			}),
	}
}

//
// Impl the handler interface.
func GetRequests(a handler.MapObject) []reconcile.Request {
	target := Target{
		Kind:      ToKind(a.Object),
		Name:      a.Meta.GetName(),
		Namespace: a.Meta.GetNamespace(),
	}
	list := []reconcile.Request{}
	for _, owner := range Map.Find(target) {
		list = append(
			list,
			reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: owner.Namespace,
					Name:      owner.Name,
				},
			})
	}

	return list
}

//
// Determine the resource Kind.
func ToKind(resource runtime.Object) string {
	gvk, err := apiutil.GVKForObject(resource, scheme.Scheme)
	if err != nil {
		return gvk.Kind
	}

	return "unknown"
}
