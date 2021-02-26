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
	Children []*Task `json:"children,omitempty"`
	// Progress.
	Progress Progress `json:"progress"`
	// Errors.
	Errors []Error `json:"error"`
	// Associated resources.
	Resources []core.ObjectReference `json:"resources,omitempty"`
	// Parallelized task.
	Parallel bool `json:"parallel"`
	// Parent.
	parent *Task
}

//
// Aggregate errors and progress.
func (r *Task) Aggregate() {
	r.aggregate()
}

//
// Aggregate errors and progress.
func (r *Task) aggregate() ([]Error, Progress) {
	r.Errors = []Error{}
	r.Progress = Progress{}
	for _, child := range r.Children {
		e, p := child.aggregate()
		r.Errors = append(r.Errors, e...)
		r.Progress.Completed += p.Completed
		r.Progress.Total += p.Total
		r.Progress.Message = strings.Join([]string{
			r.Progress.Message,
			p.Message,
		},
			";")
	}

	return r.Errors, r.Progress
}

//
// Task has failed.
func (r *Task) Failed() bool {
	r.Aggregate()
	return len(r.Errors) > 0
}

//
// Task has succeeded.
func (r *Task) Succeeded() bool {
	return !r.Failed() && r.MarkedCompleted()
}

//
// Task is running.
func (r *Task) Running() bool {
	return r.MarkedStarted() && !r.MarkedCompleted()
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
	// Path the error occurred.
	Path string `json:"path"`
	// Error.
	Error error `json:"reasons"`
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
	Index int     `json:"index"`
	Tasks []*Task `json:"tasks"`
	list  []TaskRef
	index map[string]int
}

//
// Index of path.
func (r *Pipeline) IndexOf(path string) (index int, found bool) {
	r.build()
	index, found = r.index[path]
	return
}

//
// Find task.
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
func (r *Pipeline) Next() (next *Task, done bool, err error) {
	r.build()
	if r.Index > 0 {
		r.list[r.Index-1].Task.MarkedCompleted()
	}
	if r.Index < len(r.list) {
		next = r.list[r.Index].Task
		r.Index++
	} else {
		done = true
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
	var build func(string, []*Task)
	build = func(parent string, children []*Task) {
		for _, task := range children {
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
// Join path parts.
func (r Pipeline) Join(parent, name string) (path string) {
	if parent != "" {
		path = strings.Join([]string{parent, name}, "/")
	} else {
		path = name
	}

	return
}

//
// Split path.
func (r Pipeline) Split(path string) (part []string) {
	part = strings.Split(path, "/")
	return
}
