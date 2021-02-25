package itinerary

import (
	"errors"
	liberr "github.com/konveyor/controller/pkg/error"
	core "k8s.io/api/core/v1"
	"strings"
)

//
// Errors.
var (
	TaskNotFound = errors.New("task not found")
)

//
// Pipeline task.
type Task struct {
	Timed
	// Task name.
	Name string `json:"name"`
	// Description
	Description string `json:"description,omitempty"`
	// Annotations
	Annotations map[string]string `json:"annotations,omitempty"`
	// Nested tasks.
	Children []Task `json:"children,omitempty"`
	// Progress.
	Progress Progress `json:"progress"`
	// Error.
	Error *Error `json:"error"`
	// Associated resources.
	Resources []core.ObjectReference `json:"resources,omitempty"`
	// Parallelized task.
	Parallel bool `json:"parallel"`
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
// Task reference.
type TaskRef struct {
	Path string
	Task *Task
}

//
// Pipeline.
type Pipeline struct {
	Tasks []Task `json:"tasks"`
	list  []TaskRef
	index map[string]int
}

//
// Get task.
func (r *Pipeline) Get(path string) (task *Task, err error) {
	r.build()
	ref, err := r.get(path)
	if err != nil {
		return
	}

	task = ref.Task

	return
}

//
// Next task.
func (r *Pipeline) Next(path string) (next TaskRef, done bool, err error) {
	r.build()
	done = true
	if index, found := r.index[path]; found {
		index++
		if index < len(r.list) {
			next = r.list[index]
			done = false
		}
	} else {
		err = liberr.Wrap(TaskNotFound)
	}

	return
}

//
// Get TaskRef.
func (r *Pipeline) get(path string) (ref TaskRef, err error) {
	r.build()
	if index, found := r.index[path]; found {
		if index < len(r.list) {
			ref = r.list[index]
			return
		}
	}

	err = liberr.Wrap(TaskNotFound)

	return
}

//
// Build self.
func (r *Pipeline) build() {
	if r.index != nil {
		return
	}
	r.list = []TaskRef{}
	r.index = map[string]int{}
	n := 0
	var build func(string, []Task)
	build = func(parent string, pl []Task) {
		for i := range pl {
			task := &pl[i]
			if task.Parallel {
				continue
			}
			path := r.Join(parent, task.Name)
			r.list = append(
				r.list,
				TaskRef{
					Path: path,
					Task: task,
				})
			r.index[path] = n
			n++
			build(path, task.Children)
		}
	}
	build("", r.Tasks)
}

//
// Join phase parts.
func (r Pipeline) Join(parent, name string) (phase string) {
	if parent != "" {
		phase = strings.Join([]string{parent, name}, "/")
	} else {
		phase = name
	}

	return
}

//
// Split path.
func (r Pipeline) Split(path string) (part []string) {
	part = strings.Split(path, "/")
	return
}
