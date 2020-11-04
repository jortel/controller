package runtime

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
// Pipeline.
// List of steps.
type Pipeline []Step

//
// Pipeline step.
type Step struct {
	Timed
	// Step name.
	Name string `json:"name"`
	// Description
	Description string `json:"description"`
	// Annotations
	Annotations map[string]string `json:"annotations"`
	// Nested pipeline.
	Pipeline Pipeline `json:"pipeline"`
	// Progress.
	Progress Progress `json:"progress"`
	// Error.
	Error *Error `json:"error"`
}

//
// Runtime object.
func (r Pipeline) Runtime() (runtime *Runtime) {
	runtime = &Runtime{}
	runtime.build(r)
	return
}

//
// Progress report.
type Progress struct {
	// Total units.
	Total int64 `json:"total"`
	// Number of completed units.
	Completed int64 `json:"completed"`
	// Message describing activity.
	Message string `json:"message,omitempty"`
}

//
// Error
type Error struct {
	// Phase the error occurred.
	Phase string `json:"phase"`
	// Error description.
	Reasons []string `json:"reasons"`
}

//
// Step reference.
type StepRef struct {
	Path string
	Step *Step
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
			path := r.Join(parent, step.Name)
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
func (r Runtime) Join(parent, name string) (phase string) {
	if parent != "" {
		phase = strings.Join([]string{parent, name}, "/")
	} else {
		phase = name
	}

	return
}

//
// Split path.
func (r Runtime) Split(path string) (part []string) {
	part = strings.Split(path, "/")
	return
}
