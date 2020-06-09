package watch

import (
	liberr "github.com/konveyor/controller/pkg/error"
	"github.com/konveyor/controller/pkg/ref"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
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
	Relays []*Relay
	// Watch list.
	watches []Watch
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
	var err error
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.started {
		return nil
	}
	r.watches = watch
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
	for _, relay := range r.Relays {
		err = relay.setup()
		if err != nil {
			return liberr.Wrap(err)
		}
	}
	for _, w := range watch {
		err := r.Watch(w.Object, w.Predicates...)
		if err != nil {
			return liberr.Wrap(err)
		}

	}

	go r.manager.Start(r.done)
	r.started = true

	return nil
}

//
// Add a watch.
func (r *Remote) Watch(object runtime.Object, prds ...predicate.Predicate) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.controller == nil {
		return liberr.New("not started")
	}
	return r.watch(
		Watch{
			Object:     object,
			Predicates: prds,
		})
}

//
// Add a relay.
func (r *Remote) Relay(relay *Relay, watch ...Watch) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.controller == nil {
		return liberr.New("not started")
	}
	for _, rel := range r.Relays {
		if rel.Controller == relay.Controller {
			return nil
		}
	}
	r.Relays = append(r.Relays, relay)
	for _, w := range append(r.watches, watch...) {
		err := r.watch(w)
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
	for _, relay := range r.Relays {
		relay.shutdown()
	}
}

//
// Add watch.
func (r *Remote) watch(watch Watch) error {
	for _, w := range r.watches {
		if w.Object == watch.Object {
			return nil
		}
	}
	for _, relay := range r.Relays {
		watch.Predicates = append(watch.Predicates, relay.predicate)
	}
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
// Controller relay.
type Relay struct {
	// Target controller.
	Controller controller.Controller
	// Object (resource) reconciled by the controller.
	Object Resource
	// Forward predicate.
	predicate *Forward
}

//
// Setup the relay:
//   1. create the channel
//   2. add the channel source to the controller.
//   3. build the predicate.
func (r *Relay) setup() error {
	r.predicate = &Forward{
		Channel: make(chan event.GenericEvent),
		Event: event.GenericEvent{
			Meta:   r.Object,
			Object: r.Object,
		},
	}
	err := r.Controller.Watch(
		&source.Channel{
			Source: r.predicate.Channel,
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

	close(r.predicate.Channel)
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
