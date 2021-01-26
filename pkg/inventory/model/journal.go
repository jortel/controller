package model

import (
	"encoding/json"
	liberr "github.com/konveyor/controller/pkg/error"
	"reflect"
	"sync"
)

//
// Event Actions.
var (
	Created int8 = 0x01
	Updated int8 = 0x02
	Deleted int8 = 0x04
)

//
// Model event.
type Event struct {
	// The event subject.
	Model Model
	// The event action (created|updated|deleted).
	Action int8
	// The updated model.
	Updated Model
}

//
// Event handler.
type EventHandler interface {
	// A model has been created.
	Created(Event)
	// A model has been updated.
	Updated(Event)
	// A model has been deleted.
	Deleted(Event)
	// An error has occurred delivering an event.
	Error(error)
	// An event watch has ended.
	End()
}

//
// Model event watch.
type Watch struct {
	// Model to be watched.
	Model Model
	// Event handler.
	Handler EventHandler
	// Model kind (name).
	kind string
	// Event queue.
	queue chan int8
	// Started
	started bool
	// DB
	db DBTX
	// event ID.
	eventID int64
}

//
// Match by model `kind`.
func (w *Watch) Match(kind string) bool {
	return w.kind == kind
}

//
// Queue event.
func (w *Watch) notify() {
	defer func() {
		recover()
	}()
	w.queue <- int8(0)
}

//
// Run the watch.
// Forward events to the `handler`.
func (w *Watch) Start(list *reflect.Value) {
	if w.started {
		return
	}
	run := func() {
		for i := 0; i < list.Len(); i++ {
			m := list.Index(i).Addr().Interface()
			w.Handler.Created(
				Event{
					Model:  m.(Model),
					Action: Created,
				})
		}
		list = nil
		for _ = range w.queue {
			table := Table{DB: w.db}
			history := []EventHistory{}
			err := table.List(
				&history,
				ListOptions{
					Predicate: Gt("ID", w.eventID),
					Page:      &Page{Limit: 100},
					Detail:    1,
				})
			if err != nil {
				w.Handler.Error(err)
				continue
			}
			for _, h := range history {
				w.eventID = h.ID
				if !w.Match(h.Kind) {
					continue
				}
				event := w.event(h)
				switch event.Action {
				case Created:
					w.Handler.Created(Event{
						Model:  event.Model.(Model),
						Action: event.Action,
					})
				case Updated:
					w.Handler.Updated(Event{
						Model:   event.Model.(Model),
						Updated: event.Updated.(Model),
						Action:  event.Action,
					})
				case Deleted:
					w.Handler.Deleted(Event{
						Model:  event.Model.(Model),
						Action: event.Action,
					})
				default:
					w.Handler.Error(liberr.New("unknown action"))
				}
			}

		}
		w.Handler.End()
	}

	w.started = true
	go run()
}

//
// End the watch.
func (w *Watch) End() {
	close(w.queue)
}

//
// Build an event.
func (w *Watch) event(h EventHistory) Event {
	model := newModel(w.Model)
	updated := newModel(w.Model)
	_ = json.Unmarshal([]byte(h.Model), &model)
	_ = json.Unmarshal([]byte(h.Updated), &updated)
	return Event{
		Action:  h.Action,
		Model:   model,
		Updated: updated,
	}
}

//
// Event history.
type EventHistory struct {
	// ID
	ID int64 `sql:"pk"`
	// Model kind.
	Kind string `sql:""`
	// The event subject.
	Model string `sql:""`
	// The event action.
	Action int8 `sql:""`
	// The updated model.
	Updated string `sql:""`
}

//
// Build.
func (r *EventHistory) With(model, updated Model) {
	r.Kind = Table{}.Name(model)
	b, _ := json.Marshal(model)
	r.Model = string(b)
	if updated != nil {
		b, _ = json.Marshal(updated)
		r.Updated = string(b)
	}
}

//
// Event manager.
type Journal struct {
	mutex sync.RWMutex
	// List of registered watches.
	watchList []*Watch
	// event ID.
	eventID int64
	// DB
	db DBTX
}

//
// Watch a `watch` of the specified model.
// The returned watch has not been started.
// See: Watch.Start().
func (r *Journal) Watch(model Model, handler EventHandler) (*Watch, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	watch := &Watch{
		Handler: handler,
		Model:   model,
		kind:    Table{}.Name(model),
		queue:   make(chan int8, 3),
		eventID: r.eventID,
		db:      r.db,
	}
	r.watchList = append(r.watchList, watch)
	return watch, nil
}

//
// End watch.
func (r *Journal) End(watch *Watch) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	kept := []*Watch{}
	for _, w := range r.watchList {
		if w != watch {
			kept = append(kept, w)
			continue
		}
		w.End()
	}

	r.watchList = kept
}

//
// A model has been created.
// Queue an event.
func (r *Journal) Created(db DBTX, model Model) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if len(r.watchList) == 0 {
		return
	}
	r.eventID++
	table := Table{DB: db}
	event := &EventHistory{ID: r.eventID, Action: Created}
	event.With(model, nil)
	_ = table.Insert(event)
}

//
// A model has been updated.
// Queue an event.
func (r *Journal) Updated(db DBTX, model, updated Model) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if len(r.watchList) == 0 {
		return
	}
	r.eventID++
	table := Table{DB: db}
	event := &EventHistory{ID: r.eventID, Action: Updated}
	event.With(model, updated)
	_ = table.Insert(event)
}

//
// A model has been deleted.
// Queue an event.
func (r *Journal) Deleted(db DBTX, model Model) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if len(r.watchList) == 0 {
		return
	}
	r.eventID++
	table := Table{DB: db}
	event := &EventHistory{ID: r.eventID, Action: Deleted}
	event.With(model, nil)
	_ = table.Insert(event)
}

//
// Transaction committed.
func (r *Journal) Committed() {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for _, w := range r.watchList {
		w.notify()
	}
}

//
// New model.
func  newModel(model Model) Model {
	mt := reflect.TypeOf(model)
	mv := reflect.ValueOf(model)
	switch mt.Kind() {
	case reflect.Ptr:
		mt = mt.Elem()
		mv = mv.Elem()
	}
	new := reflect.New(mt).Elem()
	return new.Addr().Interface().(Model)
}