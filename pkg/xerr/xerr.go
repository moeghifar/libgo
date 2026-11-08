// Package xerr is the typed error model shared by libgo packages: a JSON:API
// error object (https://jsonapi.org/format/#error-objects) with an HTTP
// status, an application code and a hidden internal cause. It imports only
// the standard library, so workers, CLIs and gRPC servers return the same
// errors that xttp renders, without linking an HTTP stack.
package xerr

import (
	"cmp"
	"errors"
)

// Error is a JSON:API error object. Declare them once as values and return a
// copy per occurrence with With:
//
//	var ErrUserNotFound = xerr.Error{Status: 404, Code: "USR-404", Title: "User not found"}
//
//	return ErrUserNotFound.With("no user with id 42", err)
//
// Only *Error implements error, so a shared value cannot be returned, and
// therefore cannot be mutated across requests, by accident.
type Error struct {
	Status int            // HTTP status; xttp renders it as a string, as JSON:API requires
	Code   string         // application-specific code, e.g. "USR-404"
	Title  string         // short summary, the same for every occurrence
	Detail string         // explanation of this occurrence, shown to the client
	Source *ErrorSource   // what caused it in the request
	Meta   map[string]any // non-standard extra information
	Err    error          // internal cause: logged, never sent to the client
}

// ErrorSource points at the part of the request that caused an error. Set one field.
type ErrorSource struct {
	Pointer   string `json:"pointer,omitempty"`   // JSON Pointer into the body, e.g. "/data/attributes/email"
	Parameter string `json:"parameter,omitempty"` // query parameter name
	Header    string `json:"header,omitempty"`    // request header name
}

// With returns a copy of e with Detail and the internal cause set. The
// receiver is never modified.
func (e Error) With(detail string, err error) *Error {
	e.Detail = detail
	e.Err = err
	return &e
}

func (e *Error) Error() string {
	msg := e.Title
	if e.Detail != "" {
		msg += ": " + e.Detail
	}
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

// Errors reports several problems at once, e.g. one per invalid field.
type Errors []*Error

func (es Errors) Error() string {
	if err := errors.Join(es.Unwrap()...); err != nil {
		return err.Error()
	}
	return "no errors"
}

// Unwrap returns the non-nil members.
func (es Errors) Unwrap() []error {
	out := make([]error, 0, len(es))
	for _, e := range es {
		if e != nil {
			out = append(out, e)
		}
	}
	return out
}

// Status is the status for a response carrying all of es, as JSON:API says:
// the one they share, else 400 when all are 4xx, else 500. An empty list, a
// nil member or a zero Status counts as 500: an error nobody classified is ours.
func (es Errors) Status() int {
	if len(es) == 0 {
		return 500
	}
	shared, all4xx := statusOf(es[0]), true
	for _, e := range es {
		st := statusOf(e)
		all4xx = all4xx && st >= 400 && st < 500
		if st != shared {
			shared = 0
		}
	}
	switch {
	case shared != 0:
		return shared
	case all4xx:
		return 400
	default:
		return 500
	}
}

func statusOf(e *Error) int {
	if e == nil {
		return 500
	}
	return cmp.Or(e.Status, 500)
}

// Flatten returns the *Error values err stands for: the non-nil members of
// the first Errors in the chain, else the first *Error, else nil.
func Flatten(err error) Errors {
	var es Errors
	if errors.As(err, &es) {
		out := make(Errors, 0, len(es))
		for _, e := range es {
			if e != nil {
				out = append(out, e)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	var e *Error
	if errors.As(err, &e) && e != nil {
		return Errors{e}
	}
	return nil
}

// StatusOf is the HTTP status err maps to: 0 for nil, the declared status of
// an *Error, Errors.Status for Errors, and 500 for anything else.
func StatusOf(err error) int {
	if err == nil {
		return 0
	}
	return Flatten(err).Status()
}

// CodeOf returns the Code of the first *Error in err's chain, or "".
func CodeOf(err error) string {
	var e *Error
	if errors.As(err, &e) && e != nil {
		return e.Code
	}
	return ""
}

// IsCode reports whether CodeOf(err) is code.
func IsCode(err error, code string) bool { return CodeOf(err) == code }
