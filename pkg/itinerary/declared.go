package itinerary

import (
	liberr "github.com/konveyor/controller/pkg/error"
	"github.com/konveyor/controller/pkg/itinerary/runtime"
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
// Pipeline.
// List of steps.
type Pipeline []Step

//
// Pipeline step.
type Step struct {
	// Step name.
	Name string
	// Description
	Description string
	// Annotations
	Annotations map[string]string
	// Nested pipeline.
	Pipeline Pipeline
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
func (r Itinerary) Export(predicate Predicate) (out runtime.Pipeline, err error) {
	var build func(Pipeline) runtime.Pipeline
	build = func(in Pipeline) (out runtime.Pipeline) {
		out = runtime.Pipeline{}
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
			out = append(
				out,
				runtime.Step{
					Name:        step.Name,
					Description: step.Description,
					Annotations: step.Annotations,
					Pipeline:    build(step.Pipeline),
				})
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
