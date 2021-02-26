package itinerary

import (
	"errors"
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
	// Path.
	Path string `json:"path"`
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
}

//
// Parent path.
func (r *Task) Parent() (path string) {
	parts := strings.Split(r.Path, "/")
	if len(parts) > 1 {
		path = parts[len(parts)-2]
	}

	return
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
// Pipeline.
type Pipeline struct {
	// Index of next task to be returned by next().
	Index int `json:"index"`
	// Lost of `top` level tasks.
	Tasks []*Task `json:"tasks"`
	// Flattened list of tasks in execution order.
	list []*Task
	// Task index by path.
	index map[string]*Task
}

//
// Index of path.
func (r *Pipeline) IndexOf(path string) (index int, found bool) {
	r.build()
	for i := range r.list {
		task := r.list[i]
		if task.Path == path {
			found = true
			index = i
		}
	}
	return
}

//
// Find task.
func (r *Pipeline) Get(path string) (task *Task, found bool) {
	r.build()
	task, found = r.index[path]
	return
}

//
// Current task.
func (r *Pipeline) Current() (task *Task) {
	r.build()
	if r.Index < len(r.list) {
		task = r.list[r.Index]
	} else {
		task = r.list[len(r.list)-1]
	}

	return
}

//
// Next task.
func (r *Pipeline) Next() (task *Task, done bool, err error) {
	r.build()
	task = r.Current()
	task.MarkedCompleted()
	r.Index++
	if r.Index < len(r.list) {
		task = r.list[r.Index]
	} else {
		done = true
	}

	return
}

//
// Build self.
func (r *Pipeline) build() {
	if r.index != nil {
		return
	}
	r.list = []*Task{}
	r.index = map[string]*Task{}
	var build func([]*Task)
	build = func(children []*Task) {
		for _, task := range children {
			r.index[task.Path] = task
			if task.Parallel {
				continue
			}
			r.list = append(r.list, task)
			build(task.Children)
		}
	}
	build(r.Tasks)
}
