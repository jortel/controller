package watch

import (
	liberr "github.com/konveyor/controller/pkg/error"
	"github.com/konveyor/controller/pkg/ref"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sync"
)

//
// k8s Resource.
type Resource interface {
	meta.Object
	runtime.Object
}

// Global remote map.
var Map *RmtMap

func init() {
	Map = &RmtMap{
		content: map[Key]*Remote{},
	}
}

//
// Map key.
type Key = core.ObjectReference

//
// Map a remote to CR.
type RmtMap struct {
	// Map content.
	content map[Key]*Remote
	// Protect the map.
	mutex sync.RWMutex
}

//
// Add a remote.
func (m *RmtMap) Add(object meta.Object, remote *Remote) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	key := m.key(object)
	if remote, found := m.content[key]; found {
		remote.Shutdown()
		delete(m.content, key)
	}
	m.content[key] = remote
}

//
// Delete a remote.
func (m *RmtMap) Delete(object meta.Object) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	key := m.key(object)
	if remote, found := m.content[key]; found {
		remote.Shutdown()
		delete(m.content, key)
	}
}

//
// Find a remote.
func (m *RmtMap) Find(object meta.Object) (*Remote, bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	key := m.key(object)
	remote, found := m.content[key]
	return remote, found
}

func (m *RmtMap) key(object meta.Object) Key {
	return Key{
		Kind:      ref.ToKind(object),
		Namespace: object.GetNamespace(),
		Name:      object.GetName(),
	}
}

// Represents a remote cluster.
type Remote struct {
	// A name.
	Name string
	// REST configuration
	RestCfg *rest.Config
	// Relay (forward) watch events.
	relays []*Relay
	// Watch list.
	watches []Watch
	// Routing predicate.
	router *Router
	// Manager.
	manager manager.Manager
	// Controller
	controller controller.Controller
	// Done channel.
	done chan struct{}
	// started
	started bool
	// Protect internal state.
	mutex sync.RWMutex
}

//
// Start the remote.
func (r *Remote) Start(watch ...Watch) error {
	start := func() error {
		var err error
		r.mutex.Lock()
		defer r.mutex.Unlock()
		if r.started {
			return nil
		}
		r.watches = watch
		r.router = &Router{remote: r}
		r.manager, err = manager.New(r.RestCfg, manager.Options{})
		if err != nil {
			return liberr.Wrap(err)
		}
		r.controller, err = controller.New(
			r.Name+"-R",
			r.manager,
			controller.Options{
				Reconciler: &reconciler{},
			})
		if err != nil {
			return liberr.Wrap(err)
		}

		go r.manager.Start(r.done)
		r.started = true

		return nil
	}
	err := start()
	if err != nil {
		return liberr.Wrap(err)
	}
	for _, w := range watch {
		err := r.Watch(w)
		if err != nil {
			return liberr.Wrap(err)
		}
	}

	return nil
}

//
// Shutdown the remote.
func (r *Remote) Shutdown() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	defer func() {
		recover()
	}()
	close(r.done)
	for _, relay := range r.relays {
		relay.shutdown()
	}
}

//
// Add watch.
func (r *Remote) Watch(watch Watch) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.controller == nil {
		return liberr.New("not started")
	}
	for _, w := range r.watches {
		if w.Object == watch.Object {
			return nil
		}
	}
	watch.Predicates = append(watch.Predicates, r.router)
	err := r.controller.Watch(
		&source.Kind{
			Type: watch.Object,
		},
		&nopHandler,
		watch.Predicates...)
	if err != nil {
		return liberr.Wrap(err)
	}

	return nil
}

//
// Add a relay.
func (r *Remote) Relay(relay *Relay) error {
	add := func() error {
		r.mutex.Lock()
		defer r.mutex.Unlock()
		if r.controller == nil {
			return liberr.New("not started")
		}
		for _, rel := range r.relays {
			if rel.Controller == relay.Controller {
				return nil
			}
		}
		err := relay.setup()
		if err != nil {
			return liberr.Wrap(err)
		}

		r.relays = append(r.relays, relay)

		return nil
	}
	err := add()
	if err != nil {
		return liberr.Wrap(err)
	}
	for _, w := range relay.Watch {
		err := r.Watch(Watch{Object: w.Object})
		if err != nil {
			return liberr.Wrap(err)
		}
	}

	return nil
}

//
// Controller relay.
type Relay struct {
	// Target controller.
	Controller controller.Controller
	// Object (resource) reconciled by the controller.
	Object Resource
	// Watches
	Watch []Watch
	// Forward predicate.
	forward *Forward
}

//
// Setup the relay:
//   1. create the channel
//   2. add the channel source to the controller.
//   3. build the predicate.
func (r *Relay) setup() error {
	r.forward = &Forward{
		Channel: make(chan event.GenericEvent),
		Event: event.GenericEvent{
			Meta:   r.Object,
			Object: r.Object,
		},
	}
	err := r.Controller.Watch(
		&source.Channel{
			Source: r.forward.Channel,
		},
		&handler.EnqueueRequestForObject{})

	return liberr.Wrap(err)
}

//
// Shutdown the relay.
func (r *Relay) shutdown() {
	defer func() {
		recover()
	}()

	close(r.forward.Channel)
}

//
// Watch.
type Watch struct {
	// An object (kind) watched.
	Object runtime.Object
	// Optional list of predicates.
	Predicates []predicate.Predicate
}

//
// Predicate used to forward events.
// This MUST be the last predicated listed on a watch.
type Forward struct {
	// An event channel.
	Channel chan event.GenericEvent
	// An event to be forwarded.
	Event event.GenericEvent
}

func (p *Forward) Create(e event.CreateEvent) bool {
	p.forward()
	return true
}

func (p *Forward) Update(e event.UpdateEvent) bool {
	p.forward()
	return true
}

func (p *Forward) Delete(e event.DeleteEvent) bool {
	p.forward()
	return true
}

func (p *Forward) Generic(e event.GenericEvent) bool {
	p.forward()
	return true
}

func (p Forward) forward() {
	defer func() {
		recover()
	}()
	p.Channel <- p.Event
}

//
// Router predicate
type Router struct {
	// A parent remote.
	remote *Remote
}

func (p *Router) Create(e event.CreateEvent) (rt bool) {
	for _, relay := range p.remote.relays {
	outer:
		for _, w := range relay.Watch {
			if reflect.TypeOf(w.Object) != reflect.TypeOf(e.Object) {
				continue outer
			}
			for _, wp := range w.Predicates {
				if !wp.Create(e) {
					continue outer
				}
			}
			relay.forward.Create(e)
		}
	}

	return
}

func (p *Router) Update(e event.UpdateEvent) (rt bool) {
	for _, relay := range p.remote.relays {
	outer:
		for _, w := range relay.Watch {
			if reflect.TypeOf(w.Object) != reflect.TypeOf(e.ObjectNew) {
				continue
			}
			for _, wp := range w.Predicates {
				if !wp.Update(e) {
					continue outer
				}
			}
			relay.forward.Update(e)
			break
		}
	}

	return
}

func (p *Router) Delete(e event.DeleteEvent) (rt bool) {
	for _, relay := range p.remote.relays {
	outer:
		for _, w := range relay.Watch {
			if reflect.TypeOf(w.Object) != reflect.TypeOf(e.Object) {
				continue
			}
			for _, wp := range w.Predicates {
				if !wp.Delete(e) {
					continue outer
				}
			}
			relay.forward.Delete(e)
			break
		}
	}

	return
}

func (p *Router) Generic(e event.GenericEvent) (rt bool) {
	for _, relay := range p.remote.relays {
	outer:
		for _, w := range relay.Watch {
			if reflect.TypeOf(w.Object) != reflect.TypeOf(e.Object) {
				continue
			}
			for _, wp := range w.Predicates {
				if !wp.Generic(e) {
					continue outer
				}
			}
			relay.forward.Generic(e)
			break
		}
	}

	return
}

//
// Nop reconciler.
type reconciler struct {
}

//
// Never called.
func (r *reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

// Nop handler.
var nopHandler = handler.EnqueueRequestsFromMapFunc{
	ToRequests: handler.ToRequestsFunc(
		func(a handler.MapObject) []reconcile.Request {
			return []reconcile.Request{}
		}),
}
