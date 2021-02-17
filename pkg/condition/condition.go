package condition

import (
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"reflect"
	"time"
)

// Types
const (
	ReconcileFailed = "ReconcileFailed"
	Ready           = "Ready"
)

// Status
const (
	True  = "True"
	False = "False"
)

// Category
const (
	// Errors that block Reconcile() and the `Ready` condition.
	Critical = "Critical"
	// Errors that block the `Ready` condition.
	Error = "Error"
	// Warnings that do not block the `Ready` condition.
	Warn = "Warn"
	// Required for the `Ready` condition.
	Required = "Required"
	// An advisory condition.
	Advisory = "Advisory"
)

// Condition
type Condition struct {
	// The condition type.
	Type string `json:"type"`
	// The condition status [true,false].
	Status string `json:"status"`
	// The reason for the condition or transition.
	Reason string `json:"reason,omitempty"`
	// The condition category.
	Category string `json:"category"`
	// The human readable description of the condition.
	Message string `json:"message,omitempty"`
	// When the last status transition occurred.
	LastTransitionTime v1.Time `json:"lastTransitionTime"`
	// The condition is durable - never un-staged.
	Durable bool `json:"durable,omitempty"`
	// A list of items referenced in the `Message`.
	Items []string `json:"items,omitempty"`
	// The condition has been explicitly set/updated.
	staged bool `json:"-"`
	// The condition is newly added.
	added bool `json:"-"`
	// The condition has been updated.
	updated bool `json:"-"`
}

//
// Update this condition with another's fields.
func (r *Condition) Update(other Condition) {
	r.staged = true
	if r.Equal(other) {
		return
	}
	r.Type = other.Type
	r.Status = other.Status
	r.Reason = other.Reason
	r.Category = other.Category
	r.Message = other.Message
	r.Durable = other.Durable
	r.Items = other.Items
	r.LastTransitionTime = v1.NewTime(time.Now())
	r.updated = true
}

//
// Get whether the conditions are equal.
func (r *Condition) Equal(other Condition) bool {
	return r.Type == other.Type &&
		r.Status == other.Status &&
		r.Category == other.Category &&
		r.Reason == other.Reason &&
		r.Message == other.Message &&
		r.Durable == other.Durable &&
		reflect.DeepEqual(r.Items, other.Items)
}

//
// Managed collection of conditions.
// Intended to be included in resource Status.
// -------------------
// Example:
//
// thing.Status.BeginStagingConditions()
// thing.Status.SetCondition(c)
// thing.Status.SetCondition(c)
// thing.Status.SetCondition(c)
// thing.Status.EndStagingConditions()
// thing.Status.SetReady(
//     !thing.Status.HasBlockerCondition(),
//     "Resource Ready.")
//
type Conditions struct {
	// List of conditions.
	List []Condition `json:"conditions,omitempty"`
	// Staging conditions.
	staging bool `json:"-"`
}

//
// Begin staging conditions.
func (r *Conditions) BeginStagingConditions() {
	r.staging = true
	if r.List == nil {
		return
	}
	for index := range r.List {
		condition := &r.List[index]
		condition.staged = condition.Durable
	}
}

//
// End staging conditions. Un-staged conditions are deleted.
func (r *Conditions) EndStagingConditions() (d Delta) {
	r.staging = false
	d = r.delta()
	if r.List == nil {
		return
	}
	kept := []Condition{}
	for index := range r.List {
		condition := r.List[index]
		if condition.staged {
			kept = append(kept, condition)
		}
	}
	r.List = kept
	return
}

//
// Find a condition by type.
// Staging is ignored.
func (r *Conditions) find(cndType string) *Condition {
	if r.List == nil {
		return nil
	}
	for i := range r.List {
		condition := &r.List[i]
		if condition.Type == cndType {
			return condition
		}
	}

	return nil
}

//
// Find a condition by type.
func (r *Conditions) FindCondition(cndType string) *Condition {
	if r.List == nil {
		return nil
	}
	condition := r.find(cndType)
	if condition == nil {
		return nil
	}
	if r.staging && !condition.staged {
		return nil
	}

	return condition
}

//
// Set (add/update) the specified condition to the collection.
func (r *Conditions) SetCondition(conditions ...Condition) {
	if r.List == nil {
		r.List = []Condition{}
	}
	for _, condition := range conditions {
		condition.staged = true
		found := r.find(condition.Type)
		if found == nil {
			condition.added = true
			condition.LastTransitionTime = v1.NewTime(time.Now())
			r.List = append(r.List, condition)
		} else {
			condition.updated = true
			found.Update(condition)
		}
	}
}

//
// Update conditions.
func (r *Conditions) UpdateConditions(other Conditions) {
	r.SetCondition(other.List...)
}

//
// Stage an existing condition by type.
func (r *Conditions) StageCondition(types ...string) {
	if r.List == nil {
		return
	}
	filter := make(map[string]bool)
	for _, t := range types {
		filter[t] = true
	}
	for i := range r.List {
		condition := &r.List[i]
		if _, found := filter[condition.Type]; found {
			condition.staged = true
		}
	}
}

//
// Delete conditions by type.
func (r *Conditions) DeleteCondition(types ...string) {
	if r.List == nil {
		return
	}
	filter := make(map[string]bool)
	for _, t := range types {
		filter[t] = true
	}
	kept := []Condition{}
	for i := range r.List {
		condition := r.List[i]
		_, matched := filter[condition.Type]
		if !matched {
			kept = append(kept, condition)
			continue
		}
		if r.staging {
			condition.staged = false
			kept = append(kept, condition)
		}
	}
	r.List = kept
}

//
// The collection has ALL of the specified conditions.
func (r *Conditions) HasCondition(types ...string) bool {
	if r.List == nil {
		return false
	}
	for _, cndType := range types {
		condition := r.FindCondition(cndType)
		if condition == nil || condition.Status != True {
			return false
		}
	}

	return len(types) > 0
}

//
// The collection has Any of the specified conditions.
func (r *Conditions) HasAnyCondition(types ...string) bool {
	if r.List == nil {
		return false
	}
	for _, cndType := range types {
		condition := r.FindCondition(cndType)
		if condition == nil || condition.Status != True {
			continue
		}
		return true
	}

	return false
}

//
// The collection contains any conditions with category.
func (r *Conditions) HasConditionCategory(names ...string) bool {
	if r.List == nil {
		return false
	}
	catSet := map[string]bool{}
	for _, name := range names {
		catSet[name] = true
	}
	for _, condition := range r.List {
		_, found := catSet[condition.Category]
		if !found || condition.Status != True {
			continue
		}
		if r.staging && !condition.staged {
			continue
		}
		return true
	}

	return false
}

//
// Record conditions as events.
func (r *Conditions) RecordEvents(object runtime.Object, recorder record.EventRecorder) {
	for _, cnd := range r.List {
		if cnd.Status != True || !cnd.staged {
			continue
		}
		eventType := ""
		switch cnd.Category {
		case Critical,
			Error,
			Warn:
			eventType = core.EventTypeWarning
		default:
			eventType = core.EventTypeNormal
		}
		recorder.Event(object, eventType, cnd.Type, cnd.Message)
	}
}

//
// The collection contains a `Critical` error condition.
// Resource reconcile() should not continue.
func (r *Conditions) HasCriticalCondition(category ...string) bool {
	return r.HasConditionCategory(Critical)
}

//
// The collection contains an `Error` condition.
func (r *Conditions) HasErrorCondition(category ...string) bool {
	return r.HasConditionCategory(Error)
}

//
// The collection contains a `Warn` condition.
func (r *Conditions) HasWarnCondition(category ...string) bool {
	return r.HasConditionCategory(Warn)
}

//
// The collection contains a `Ready` blocker condition.
func (r *Conditions) HasBlockerCondition() bool {
	return r.HasConditionCategory(Critical, Error)
}

//
// The collection contains the `Ready` condition.
func (r *Conditions) IsReady() bool {
	condition := r.FindCondition(Ready)
	if condition == nil || condition.Status != True {
		return false
	}

	return true
}

//
// Build delta.
func (r *Conditions) delta() Delta {
	d := Delta{}
	for _, cnd := range r.List {
		if cnd.added {
			d.Added = append(d.Added, cnd)
			continue
		}
		if cnd.updated {
			d.Updated = append(d.Updated, cnd)
			continue
		}
		if !cnd.staged {
			d.Deleted = append(d.Deleted, cnd)
			continue
		}
	}

	return d
}

//
// Delta report.
type Delta struct {
	// conditions added.
	Added []Condition `json:"added"`
	// conditions updated.
	Updated []Condition `json:"updated"`
	// conditions deleted.
	Deleted []Condition `json:"deleted"`
}

//
// Total number of chances.
func (r *Delta) Len() int {
	return len(r.Updated) + len(r.Updated) + len(r.Deleted)
}

//
// The delta is empty.
func (r *Delta) Empty() bool {
	return r.Len() == 0
}
