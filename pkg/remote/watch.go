package remote

import (
	liberr "github.com/konveyor/controller/pkg/error"
	"github.com/konveyor/controller/pkg/ref"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/workqueue"
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

//
// Global container
var Manager *Container

func init() {
	Manager = &Container{
		remote: map[Key]*Remote{},
	}
}

//
// Map key.
type Key = core.ObjectReference

//
// Container of Remotes.
type Container struct {
	// Map content.
	remote map[Key]*Remote
	// Protect the map.
	mutex sync.RWMutex
}

//
// Ensure the remote is in the container
// and started.
// When already contained:
//   different rest configuration:
//     - transfer workload to the new remote.
//     - shutdown old remote.
//     - start new remote.
//   same reset configuration:
//     - nothing
func (r *Container) Ensure(owner meta.Object, new *Remote) (*Remote, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	key := r.key(owner)
	if remote, found := r.remote[key]; found {
		if remote.Equals(new) {
			return remote, nil
		}
		new.TakeWorkload(remote)
		remote.Shutdown()
	}
	err := new.Start()
	if err != nil {
		return new, liberr.Wrap(err)
	}

	r.remote[key] = new

	return new, nil
}

//
// Add a remote.
func (r *Container) Add(owner meta.Object, new *Remote) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	key := r.key(owner)
	r.remote[key] = new
}

//
// Delete a remote.
func (r *Container) Delete(owner meta.Object) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	key := r.key(owner)
	delete(r.remote, key)
}

//
// Find a remote.
func (r *Container) Find(owner meta.Object) (*Remote, bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	key := r.key(owner)
	remote, found := r.remote[key]
	return remote, found
}

//
// Ensure a resource is being watched.
func (r *Container) EnsureWatch(object meta.Object, watch *Watch) error {
	remote, found := r.Find(object)
	if !found {
		remote = &Remote{}
	}

	remote.EnsureWatch(watch)

	return nil
}

//
// Ensure a resource is being watched and relayed
// to the specified controller.
func (r *Container) EnsureRelay(object meta.Object, relay *Relay) error {
	remote, found := r.Find(object)
	if !found {
		remote = &Remote{}
	}

	remote.EnsureRelay(relay)

	return nil
}

//
// End a relay.
// Must have:
//   Relay.Controller
//   Relay.Watch.Subject.
func (r *Container) EndRelay(object meta.Object, relay *Relay) {
	remote, found := r.Find(object)
	if !found {
		return
	}

	remote.EndRelay(relay)
}

func (r *Container) key(object meta.Object) Key {
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
	// Protect internal state.
	mutex sync.RWMutex
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
	r.mutex.Lock()
	defer r.mutex.Unlock()
	var err error
	if r.started {
		return nil
	}
	if r.RestCfg == nil {
		return liberr.New("not configured")
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
// Take workloads.
// This will Reset() the other.
func (r *Remote) TakeWorkload(other *Remote) {
	for _, watch := range other.watch {
		watch.reset()
		r.EnsureWatch(watch)
	}
	for _, relay := range other.relay {
		relay.reset()
		r.EnsureRelay(relay)
	}

	other.Reset()
}

//
// Is the same remote.
// Compared based on REST configuration.
func (r *Remote) Equals(other *Remote) bool {
	return reflect.DeepEqual(
		other.RestCfg,
		r.RestCfg)
}

//
// Reset workloads.
func (r *Remote) Reset() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.watch = []*Watch{}
	r.relay = []*Relay{}
}

//
// Shutdown the remote.
func (r *Remote) Shutdown() {
	defer func() {
		recover()
	}()
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for _, relay := range r.relay {
		relay.shutdown()
	}
	close(r.done)
	r.started = false
}

//
// Watch.
func (r *Remote) EnsureWatch(watch *Watch) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	err := r.ensureWatch(watch)
	if err != nil {
		return liberr.Wrap(err)
	}

	return nil
}

//
// Relay.
func (r *Remote) EnsureRelay(relay *Relay) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.hasRelay(relay) {
		return nil
	}
	err := relay.Install()
	if err != nil {
		return liberr.Wrap(err)
	}
	watch := &Watch{
		Subject: relay.Subject(),
	}
	err = r.ensureWatch(watch)
	if err != nil {
		return liberr.Wrap(err)
	}

	r.relay = append(r.relay, relay)

	return nil
}

//
// End relay.
// Must have:
//   Relay.Controller,
//   Relay.Watch.Subject.
func (r *Remote) EndRelay(relay *Relay) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for i, found := range r.relay {
		if found.Match(relay) {
			r.relay = append(r.relay[:i], r.relay[i+1:]...)
			found.shutdown()
		}
	}
}

//
// Ensure watch.
// Not re-entrant.
func (r *Remote) ensureWatch(watch *Watch) error {
	if r.hasWatch(watch.Subject) {
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
	// Subject (watched) resource.
	Target Resource
	// Controller (target)
	Controller controller.Controller
	// Watch
	Watch Watch
	// Channel to relay events.
	channel chan event.GenericEvent
	// stop
	stop chan struct{}
	// installed
	installed bool
}

func (r *Relay) Subject() Resource {
	return r.Watch.Subject
}

func (r *Relay) Predicates() []predicate.Predicate {
	return r.Watch.Predicates
}

func (r *Relay) reset() {
}

//
// Install
func (r *Relay) Install() error {
	if r.installed {
		return nil
	}
	h := &handler.EnqueueRequestForObject{}
	err := r.Controller.Watch(r, h, r.Watch.Predicates...)
	if err != nil {
		return liberr.Wrap(err)
	}

	r.installed = true

	return nil
}

//
// Match.
func (r *Relay) Match(other *Relay) bool {
	return r.Watch.Match(other.Watch.Subject) &&
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
		Meta:   r.Target,
		Object: r.Target,
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
	// A resource to be watched.
	Subject Resource
	// Predicates.
	Predicates []predicate.Predicate
	// Started.
	started bool
}

func (w *Watch) reset() {
	w.started = false
}

//
// Match
func (w *Watch) Match(r Resource) bool {
	return ref.ToKind(w.Subject) == ref.ToKind(r)
}

//
// Start watch.
func (w *Watch) start(remote *Remote) error {
	if w.started || !remote.started {
		return nil
	}
	predicates := append(w.Predicates, &Forward{remote: remote})
	err := remote.controller.Watch(
		&source.Kind{Type: w.Subject},
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
	subject := Watch{Subject: e.Object.(Resource)}
	for _, relay := range p.remote.relay {
		if subject.Match(relay.Subject()) {
			relay.send()
		}
	}

	return false
}

func (p *Forward) Update(e event.UpdateEvent) bool {
	subject := Watch{Subject: e.ObjectNew.(Resource)}
	for _, relay := range p.remote.relay {
		if subject.Match(relay.Subject()) {
			relay.send()
		}
	}

	return false
}

func (p *Forward) Delete(e event.DeleteEvent) bool {
	subject := Watch{Subject: e.Object.(Resource)}
	for _, relay := range p.remote.relay {
		if subject.Match(relay.Subject()) {
			relay.send()
		}
	}

	return false
}

func (p *Forward) Generic(e event.GenericEvent) bool {
	subject := Watch{Subject: e.Object.(Resource)}
	for _, relay := range p.remote.relay {
		if subject.Match(relay.Subject()) {
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
