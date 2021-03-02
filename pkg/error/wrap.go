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
		le.addContext(context)
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
	newError := &Error{
		stack:   stack,
		wrapped: err,
	}

	newError.addContext(context)

	return newError
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
// Error.
// Wraps a root cause error and captures
// the stack.
type Error struct {
	// Original error.
	wrapped error
	// Context.
	context [][]interface{}
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
func (e Error) Context() (context []map[interface{}]interface{}) {
	if e.context == nil {
		return
	}
	context = []map[interface{}]interface{}{}
	for _, l := range e.context {
		mp := map[interface{}]interface{}{}
		i := 0
		for {
			if (i + 1) < len(l) {
				mp[l[i]] = l[i+1]
				i += 2
			} else {
				break
			}
		}
		context = append(context, mp)
	}

	return
}

//
// Unwrap the error.
func (e Error) Unwrap() error {
	return Unwrap(e.wrapped)
}

//
// Add context.
func (e *Error) addContext(kvpair []interface{}) {
	if len(kvpair) > 0 {
		e.context = append(e.context, kvpair)
	}
}