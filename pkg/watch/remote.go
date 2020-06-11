package watch

import (
	liberr "github.com/konveyor/controller/pkg/error"
	"github.com/konveyor/controller/pkg/ref"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/workqueue"
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

//
var Manager *Router

func init() {
	Manager = &Router{
		remote: map[Key]*Remote{},
	}
}

//
// Map key.
type Key = core.ObjectReference

type Router struct {
	// Map content.
	remote map[Key]*Remote
	// Protect the map.
	mutex sync.RWMutex
}

//
// Add a remote.
func (r *Router) Add(object meta.Object, remote *Remote) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	key := r.key(object)
	if remote, found := r.remote[key]; found {
		remote.Shutdown()
		delete(r.remote, key)
	}
	r.remote[key] = remote
}

//
// Delete a remote.
func (r *Router) Delete(object meta.Object) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	key := r.key(object)
	if remote, found := r.remote[key]; found {
		remote.Shutdown()
		delete(r.remote, key)
	}
}

//
// Find a remote.
func (r *Router) Find(object meta.Object) (*Remote, bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	key := r.key(object)
	remote, found := r.remote[key]
	return remote, found
}

func (r *Router) key(object meta.Object) Key {
	return Key{
		Kind:      ref.ToKind(object),
		Namespace: object.GetNamespace(),
		Name:      object.GetName(),
	}
}

func (r *Router) Watch(
	remote meta.Object,
	subject Resource,
	cnt controller.Controller,
	object Resource,
	predicates ...predicate.Predicate) error {
	//
	rmt, found := r.Find(remote)
	if !found {
		rmt = &Remote{}
	}
	relay := &Relay{
		Controller: cnt,
		Subject:    subject,
		Object:     object,
	}

	rmt.Relay(relay, predicates...)

	return nil
}

//
// End a watch.
func (r *Router) EndWatch(remote meta.Object, subject Resource, cnt controller.Controller) {
	rmt, found := r.Find(remote)
	if found {
		rmt.EndRelay(cnt, subject)
	}
}

// Represents a remote cluster.
type Remote struct {
	// A name.
	Name string
	// REST configuration
	RestCfg *rest.Config
	// Relay list.
	relay []*Relay
	// Watch list.
	watch []*Watch
	// Manager.
	manager manager.Manager
	// Controller
	controller controller.Controller
	// Done channel.
	done chan struct{}
	// started
	started bool
}

//
// Start the remote.
func (r *Remote) Start() error {
	var err error
	if r.started {
		return nil
	}
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
	for _, watch := range r.watch {
		err := watch.start(r)
		if err != nil {
			return liberr.Wrap(err)
		}
	}

	go r.manager.Start(r.done)
	r.started = true

	return nil
}

//
// Shutdown the remote.
func (r *Remote) Shutdown() {
	defer func() {
		recover()
	}()
	close(r.done)
}

//
// Watch.
func (r *Remote) Watch(watch *Watch) error {
	if r.hasWatch(watch.Object) {
		return nil
	}
	r.watch = append(r.watch, watch)
	err := watch.start(r)
	if err != nil {
		return liberr.Wrap(err)
	}

	return nil
}

//
// Relay.
func (r *Remote) Relay(relay *Relay, prds ...predicate.Predicate) error {
	if r.hasRelay(relay) {
		return nil
	}
	err := relay.Install(prds)
	if err != nil {
		return liberr.Wrap(err)
	}
	watch := &Watch{Object: relay.Subject}
	err = r.Watch(watch)
	if err != nil {
		return liberr.Wrap(err)
	}

	r.relay = append(r.relay, relay)

	return nil
}

//
// End relay.
func (r *Remote) EndRelay(cnt controller.Controller, subject Resource) {
	relay := &Relay{Controller: cnt, Subject: subject}
	for i, found := range r.relay {
		if found.Match(relay) {
			r.relay = append(r.relay[:i], r.relay[i+1:]...)
			found.shutdown()
		}
	}
}

//
// Has a watch.
func (r *Remote) hasWatch(object Resource) bool {
	for _, found := range r.watch {
		if found.Match(object) {
			return true
		}
	}

	return false
}

//
// Has a watch.
func (r *Remote) hasRelay(relay *Relay) bool {
	for _, found := range r.relay {
		if found.Match(relay) {
			return true
		}
	}

	return false
}

//
// Controller relay.
type Relay struct {
	base source.Channel
	// An object to be included in the relayed event.
	Object Resource
	// Subject (watched) resource.
	Subject Resource
	// Controller (target)
	Controller controller.Controller
	// Channel to relay events.
	channel chan event.GenericEvent
	// stop
	stop chan struct{}
	// installed
	installed bool
}

//
// Install
func (r *Relay) Install(prds []predicate.Predicate) error {
	if r.installed {
		return nil
	}
	h := &handler.EnqueueRequestForObject{}
	err := r.Controller.Watch(r, h, prds...)
	if err != nil {
		return liberr.Wrap(err)
	}

	r.installed = true

	return nil
}

//
// Match.
func (r *Relay) Match(other *Relay) bool {
	return ref.ToKind(r.Subject) == ref.ToKind(other.Subject) &&
		r.Controller == other.Controller
}

//
// Start the relay.
func (r *Relay) Start(
	handler handler.EventHandler,
	queue workqueue.RateLimitingInterface,
	predicates ...predicate.Predicate) error {
	r.channel = make(chan event.GenericEvent)
	r.stop = make(chan struct{})
	r.base.InjectStopChannel(r.stop)
	r.base.Source = r.channel
	err := r.base.Start(handler, queue, predicates...)
	if err != nil {
		return liberr.Wrap(err)
	}

	return nil
}

//
// Send the event.
func (r *Relay) send() {
	defer func() {
		recover()
	}()
	event := event.GenericEvent{
		Meta:   r.Object,
		Object: r.Object,
	}

	r.channel <- event
}

//
// Shutdown the relay.
func (r *Relay) shutdown() {
	defer func() {
		recover()
	}()
	close(r.stop)
	close(r.channel)
}

//
// Watch.
type Watch struct {
	// An object (kind) watched.
	Object Resource
	// Predicates.
	Predicates []predicate.Predicate
	// Started.
	started bool
}

//
// Match
func (w *Watch) Match(r Resource) bool {
	return ref.ToKind(w.Object) == ref.ToKind(r)
}

//
// Start watch.
func (w *Watch) start(remote *Remote) error {
	if w.started {
		return nil
	}
	predicates := append(w.Predicates, &Forward{remote: remote})
	err := remote.controller.Watch(
		&source.Kind{Type: w.Object},
		&nopHandler,
		predicates...)
	if err != nil {
		return liberr.Wrap(err)
	}

	w.started = true

	return nil
}

//
// Forward predicate
type Forward struct {
	// A parent remote.
	remote *Remote
}

func (p *Forward) Create(e event.CreateEvent) bool {
	subject := Watch{Object: e.Object.(Resource)}
	for _, relay := range p.remote.relay {
		if subject.Match(relay.Subject) {
			relay.send()
		}
	}

	return false
}

func (p *Forward) Update(e event.UpdateEvent) bool {
	subject := Watch{Object: e.ObjectNew.(Resource)}
	for _, relay := range p.remote.relay {
		if subject.Match(relay.Subject) {
			relay.send()
		}
	}

	return false
}

func (p *Forward) Delete(e event.DeleteEvent) bool {
	subject := Watch{Object: e.Object.(Resource)}
	for _, relay := range p.remote.relay {
		if subject.Match(relay.Subject) {
			relay.send()
		}
	}

	return false
}

func (p *Forward) Generic(e event.GenericEvent) bool {
	subject := Watch{Object: e.Object.(Resource)}
	for _, relay := range p.remote.relay {
		if subject.Match(relay.Subject) {
			relay.send()
		}
	}

	return false
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
