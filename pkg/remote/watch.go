package remote

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

//
// Global container
var RemoteContainer *Container

func init() {
	RemoteContainer = &Container{
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
	// Protect the state.
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
	r.remote[r.key(owner)] = new
}

//
// Delete a remote.
func (r *Container) Delete(owner meta.Object) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	delete(r.remote, r.key(owner))
}

//
// Find a remote.
func (r *Container) Find(owner meta.Object) (*Remote, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	remote, found := r.remote[r.key(owner)]
	return remote, found
}

//
// Ensure a resource is being watched.
func (r *Container) EnsureWatch(owner meta.Object, watch Watch) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	key := r.key(owner)
	remote, found := r.remote[key]
	if !found {
		remote = &Remote{}
		r.remote[key] = remote
	}
	err := remote.EnsureWatch(watch)
	if err != nil {
		return liberr.Wrap(err)
	}

	return nil
}

//
// Ensure a resource is being watched and relayed
// to the specified controller.
func (r *Container) EnsureRelay(owner meta.Object, relay *Relay) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	key := r.key(owner)
	remote, found := r.remote[key]
	if !found {
		remote = &Remote{}
		r.remote[key] = remote
	}
	err := remote.EnsureRelay(relay)
	if err != nil {
		return liberr.Wrap(err)
	}

	return nil
}

//
// End a relay.
// Must have: Relay.Channel
func (r *Container) EndRelay(owner meta.Object, relay *Relay) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	key := r.key(owner)
	remote, found := r.remote[key]
	if !found {
		return
	}

	remote.EndRelay(relay)
}

//
// Ensure relay group.
func (r *Container) EnsureRelayDefinition(def *RelayDefinition) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for key, remote := range r.remote {
		if !def.hasRemote(key, r.key) {
			remote.EndRelay(
				&Relay{
					Channel: def.Channel,
				})
		}
	}
	for _, wDef := range def.Watch {
		key := r.key(wDef.RemoteOwner)
		remote, found := r.remote[key]
		if !found {
			remote = &Remote{}
			r.remote[key] = remote
		}
		relay := &Relay{
			Channel: def.Channel,
			Target:  def.Target,
			Watch:   wDef.Watch,
		}
		err := remote.EnsureRelay(relay)
		if err != nil {
			return liberr.Wrap(err)
		}
	}

	return nil
}

func (r *Container) key(owner meta.Object) Key {
	return Key{
		Kind:      ref.ToKind(owner),
		Namespace: owner.GetNamespace(),
		Name:      owner.GetName(),
	}
}

type WatchDefinition struct {
	RemoteOwner meta.Object
	Watch       []Watch
}

type RelayDefinition struct {
	Channel chan event.GenericEvent
	Target  Resource
	Watch   []WatchDefinition
}

func (r *RelayDefinition) hasRemote(key Key, fn func(meta.Object) Key) bool {
	for _, w := range r.Watch {
		if fn(w.RemoteOwner) == key {
			return true
		}
	}

	return false
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
	close(r.done)
	r.started = false
}

//
// Watch.
func (r *Remote) EnsureWatch(watch Watch) error {
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
	if rl, found := r.findRelay(relay); found {
		rl.merge(relay)
		relay = rl
	} else {
		r.relay = append(r.relay, relay)
	}
	for _, watch := range relay.Watch {
		err := r.ensureWatch(Watch{Subject: watch.Subject})
		if err != nil {
			return liberr.Wrap(err)
		}
	}

	return nil
}

//
// End relay.
// Must have:
//   Relay.Channel,
func (r *Remote) EndRelay(relay *Relay) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for i, found := range r.relay {
		if found.Match(relay) {
			r.relay = append(r.relay[:i], r.relay[i+1:]...)
		}
	}
}

//
// Ensure watch.
// Not re-entrant.
func (r *Remote) ensureWatch(watch Watch) error {
	if w, found := r.findWatch(watch.Subject); found {
		w.merge(watch)
		return nil
	}
	r.watch = append(r.watch, &watch)
	err := watch.start(r)
	if err != nil {
		return liberr.Wrap(err)
	}

	return nil
}

//
// Take workloads.
// This will Reset() the other.
func (r *Remote) TakeWorkload(other *Remote) {
	for _, watch := range other.watch {
		watch.reset()
		r.EnsureWatch(*watch)
	}
	for _, relay := range other.relay {
		relay.reset()
		r.EnsureRelay(relay)
	}

	other.Reset()
}

//
// Find a watch.
func (r *Remote) findWatch(object Resource) (*Watch, bool) {
	for _, w := range r.watch {
		if w.Match(object) {
			return w, true
		}
	}

	return nil, false
}

//
// Find a relay.
func (r *Remote) findRelay(relay *Relay) (*Relay, bool) {
	for _, rl := range r.relay {
		if rl.Match(relay) {
			return rl, true
		}
	}

	return nil, false
}

//
// Controller relay.
type Relay struct {
	base source.Channel
	// Subject (watched) resource.
	Target Resource
	// Relay channel
	Channel chan event.GenericEvent
	// Watches
	Watch []Watch
	// stop
	stop chan struct{}
}

func (r *Relay) reset() {
}

//
// Match.
func (r *Relay) Match(other *Relay) bool {
	return r.Channel == other.Channel
}

//
// Send the event.
func (r *Relay) send() {
	defer func() {
		recover()
	}()
	r.Channel <- event.GenericEvent{
		Meta:   r.Target,
		Object: r.Target,
	}
}

//
// Merge another relay.
func (r *Relay) merge(other *Relay) {
	for _, watch := range other.Watch {
		if w, found := r.findWatch(watch); !found {
			r.Watch = append(r.Watch, watch)
		} else {
			w.merge(watch)
		}
	}
}

//
// Find a watch
func (r *Relay) findWatch(watch Watch) (*Watch, bool) {
	for _, w := range r.Watch {
		if w.Match(watch.Subject) {
			return &w, true
		}
	}

	return nil, false
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
// Merge another watch.
func (w *Watch) merge(other Watch) {
	w.Predicates = other.Predicates
}

//
// Create approved by predicates.
func (w *Watch) create(e event.CreateEvent) bool {
	for _, p := range w.Predicates {
		if !p.Create(e) {
			return false
		}
	}

	return true
}

//
// Update approved by predicates.
func (w *Watch) update(e event.UpdateEvent) bool {
	for _, p := range w.Predicates {
		if !p.Update(e) {
			return false
		}
	}

	return true
}

//
// Delete approved by predicates.
func (w *Watch) delete(e event.DeleteEvent) bool {
	for _, p := range w.Predicates {
		if !p.Delete(e) {
			return false
		}
	}

	return true
}

//
// Create approved by predicates.
func (w *Watch) generic(e event.GenericEvent) bool {
	for _, p := range w.Predicates {
		if !p.Generic(e) {
			return false
		}
	}

	return true
}

//
// Forward predicate
type Forward struct {
	// A parent remote.
	remote *Remote
}

func (p *Forward) Create(e event.CreateEvent) bool {
	p.remote.mutex.RLock()
	defer p.remote.mutex.RUnlock()
	subject := Watch{Subject: e.Object.(Resource)}
	for _, relay := range p.remote.relay {
		for _, watch := range relay.Watch {
			if !subject.Match(watch.Subject) || !watch.create(e) {
				continue
			}
			relay.send()
		}
	}

	return false
}

func (p *Forward) Update(e event.UpdateEvent) bool {
	p.remote.mutex.RLock()
	defer p.remote.mutex.RUnlock()
	subject := Watch{Subject: e.ObjectNew.(Resource)}
	for _, relay := range p.remote.relay {
		for _, watch := range relay.Watch {
			if !subject.Match(watch.Subject) || !watch.update(e) {
				continue
			}
			relay.send()
		}
	}

	return false
}

func (p *Forward) Delete(e event.DeleteEvent) bool {
	p.remote.mutex.RLock()
	defer p.remote.mutex.RUnlock()
	subject := Watch{Subject: e.Object.(Resource)}
	for _, relay := range p.remote.relay {
		for _, watch := range relay.Watch {
			if !subject.Match(watch.Subject) || !watch.delete(e) {
				continue
			}
			relay.send()
		}
	}

	return false
}

func (p *Forward) Generic(e event.GenericEvent) bool {
	p.remote.mutex.RLock()
	defer p.remote.mutex.RUnlock()
	subject := Watch{Subject: e.Object.(Resource)}
	for _, relay := range p.remote.relay {
		for _, watch := range relay.Watch {
			if !subject.Match(watch.Subject) || !watch.generic(e) {
				continue
			}
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
