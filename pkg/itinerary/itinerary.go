package itinerary

import (
	liberr "github.com/konveyor/controller/pkg/error"
	pathlib "path"
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
// Step.
type Step struct {
	// Step name.
	Name string
	// Description
	Description string
	// Annotations
	Annotations map[string]string
	// Children (nested).
	Children []Step
	// Any of these conditions be satisfied for
	// the step to be included.
	All Flag
	// All of these conditions be satisfied for
	// the step to be included.
	Any Flag
}

// Itinerary.
// Proposed tree of ordered, conditional steps.
type Itinerary []Step

//
//
func (r Itinerary) Pipeline(predicate Predicate) (out Pipeline, err error) {
	var build func(*Task, []Step) []*Task
	build = func(parent *Task, in []Step) (out []*Task) {
		out = []*Task{}
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
			path := step.Name
			if parent != nil {
				path = pathlib.Join(parent.Path, step.Name)
			}
			task := &Task{
				Name:        step.Name,
				Path:        path,
				Description: step.Description,
				Annotations: step.Annotations,
				Managed:     true,
			}
			task.Progress.Total = 1
			task.Children = build(task, step.Children)
			out = append(out, task)
		}
		return
	}

	out = Pipeline{Tasks: build(nil, r)}

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
