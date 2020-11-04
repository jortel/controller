package itinerary

import (
	"errors"
	liberr "github.com/konveyor/controller/pkg/error"
	"strings"
)

//
// Errors.
var (
	StepNotFound = errors.New("step not found")
)

//
// Predicate flag.
type Flag = int16

//
// Predicate.
// Flags delegated to the predicate.
type Predicate interface {
	// Evaluate the condition.
	// Returns (true) when the step should be included.
	Evaluate(Flag) (bool, error)
}

//
// Progress report.
type Progress struct {
	// Completed units.
	Completed int64 `json:"completed"`
	// Total units.
	Total int64 `json:"total"`
}

//
// Error
type Error struct {
	Phase   string
	Reasons []string
}

//
// Step reference.
type StepRef struct {
	Path string
	*Step
}

//
// Runtime pipeline.
type Runtime struct {
	list  []StepRef
	index map[string]int
}

//
// Get step.
func (r *Runtime) Get(path string) (step *Step, err error) {
	ref, err := r.get(path)
	if err != nil {
		err = liberr.Wrap(err)
		return
	}

	step = ref.Step

	return
}

//
// Next step.
func (r *Runtime) Next(path string) (next StepRef, done bool, err error) {
	done = true
	if index, found := r.index[path]; found {
		index++
		if index < len(r.list) {
			next = r.list[index]
			done = false
		}
	} else {
		err = liberr.Wrap(StepNotFound)
	}

	return
}

//
// Get stepRef.
func (r *Runtime) get(path string) (ref StepRef, err error) {
	if index, found := r.index[path]; found {
		if index < len(r.list) {
			ref = r.list[index]
			return
		}
	}

	err = liberr.Wrap(StepNotFound)

	return
}

// Build self.
func (r *Runtime) build(pipeline Pipeline) {
	r.list = []StepRef{}
	r.index = map[string]int{}
	n := 0
	var build func(string, Pipeline)
	build = func(parent string, pl Pipeline) {
		for i := range pl {
			step := &pl[i]
			path := r.join(parent, step.Name)
			r.list = append(
				r.list,
				StepRef{
					Path: path,
					Step: step,
				})
			r.index[path] = n
			n++
			build(path, step.Pipeline)
		}
	}
	build("", pipeline)
}

//
// Join phase parts.
func (r Runtime) join(parent, name string) (phase string) {
	if parent != "" {
		phase = strings.Join([]string{parent, name}, "/")
	} else {
		phase = name
	}

	return
}

//
// Pipeline.
// List of steps.
type Pipeline []Step

//
// Runtime object.
func (r Pipeline) Runtime() (runtime *Runtime) {
	runtime = &Runtime{}
	runtime.build(r)
	return
}

//
// Itinerary step.
type Step struct {
	Timed
	// Step name.
	Name string
	// Description
	Description string
	// Nested pipeline.
	Pipeline Pipeline
	// Progress report.
	Progress Progress
	// Error.
	Error *Error
	// Any of these conditions be satisfied for
	// the step to be included.
	All Flag
	// All of these conditions be satisfied for
	// the step to be included.
	Any Flag
}

//
// An itinerary.
// List of conditional steps.
type Itinerary struct {
	// Name.
	Name string
	// Pipeline (list) of steps.
	Pipeline
}

//
//
func (r Itinerary) Export(predicate Predicate) (out Pipeline, err error) {
	var build func(Pipeline) Pipeline
	build = func(in Pipeline) (out Pipeline) {
		out = Pipeline{}
		for _, step := range in {
			pTrue, pErr := r.hasAny(predicate, step)
			if pErr != nil {
				err = liberr.Wrap(pErr)
				return
			}
			if !pTrue {
				continue
			}
			pTrue, pErr = r.hasAll(predicate, step)
			if pErr != nil {
				err = liberr.Wrap(pErr)
				return
			}
			if !pTrue {
				continue
			}
			out = append(out, step)
			build(step.Pipeline)
		}
		return
	}

	out = build(r.Pipeline)

	return
}

//
// The step has satisfied ANY of the predicates.
func (r *Itinerary) hasAny(predicate Predicate, step Step) (pTrue bool, err error) {
	for i := 0; i < 16; i++ {
		flag := Flag(1 << i)
		if (step.Any & flag) == 0 {
			continue
		}
		if predicate == nil {
			continue
		}
		pTrue, err = predicate.Evaluate(flag)
		if pTrue || err != nil {
			return
		}
	}

	pTrue = true
	return
}

//
// The step has satisfied ALL of the predicates.
func (r *Itinerary) hasAll(predicate Predicate, step Step) (pTrue bool, err error) {
	for i := 0; i < 16; i++ {
		flag := Flag(1 << i)
		if (step.All & flag) == 0 {
			continue
		}
		if predicate == nil {
			continue
		}
		pTrue, err = predicate.Evaluate(flag)
		if !pTrue || err != nil {
			return
		}
	}

	pTrue = true
	return
}
