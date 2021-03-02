package error

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
)

//
// Create a new wrapped error.
func New(m string) error {
	return Wrap(errors.New(m))
}

//
// Wrap an error.
// Returns `err` when err is `nil` or *Error.
func Wrap(err error, context ...interface{}) error {
	if err == nil {
		return err
	}
	if le, cast := err.(*Error); cast {
		le.context = append(le.context, context...)
		return le
	}
	bfr := make([]uintptr, 50)
	n := runtime.Callers(2, bfr[:])
	frames := runtime.CallersFrames(bfr[:n])
	stack := []string{""}
	for {
		f, hasNext := frames.Next()
		frame := fmt.Sprintf(
			"%s()\n\t%s:%d",
			f.Function,
			f.File,
			f.Line)
		stack = append(stack, frame)
		if !hasNext {
			break
		}
	}
	return &Error{
		stack:   stack,
		context: context,
		wrapped: err,
	}
}

//
// Unwrap an error.
// Returns: the original error when not wrapped.
func Unwrap(err error) (out error) {
	if err == nil {
		return
	}
	out = err
	for {
		if wrapped, cast := out.(interface{ Unwrap() error }); cast {
			out = wrapped.Unwrap()
		} else {
			break
		}
	}

	return
}

//
// Key/Value Map.
type Map map[string]interface{}

//
// Error.
// Wraps a root cause error and captures
// the stack.
type Error struct {
	// Original error.
	wrapped error
	// Context.
	context []interface{}
	// Stack.
	stack []string
}

//
// Error description.
func (e Error) Error() string {
	return e.wrapped.Error()
}

//
// Error stack trace.
// Format:
//   package.Function()
//     file:line
//   package.Function()
//     file:line
//   ...
func (e Error) Stack() string {
	return strings.Join(e.stack, "\n")
}

//
// Get `context` key/value pairs.
func (e Error) Context() (list []interface{}) {
	return e.context
}

//
// Unwrap the error.
func (e Error) Unwrap() error {
	return Unwrap(e.wrapped)
}
