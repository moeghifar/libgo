package xttp

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

// MIMEJSONAPI is the JSON:API media type every error response is sent with.
const MIMEJSONAPI = "application/vnd.api+json"

// Error is a JSON:API error object (https://jsonapi.org/format/#error-objects).
// Declare them once as values and return a copy per request with With:
//
//	var ErrUserNotFound = xttp.Error{Status: 404, Code: "USR-404", Title: "User not found"}
//
//	return ErrUserNotFound.With("no user with id 42", err)
//
// Only *Error implements error, so a shared value cannot be returned, and
// therefore cannot be mutated across requests, by accident.
type Error struct {
	Status int            // HTTP status; rendered as a string, as JSON:API requires
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

func (es Errors) Error() string { return errors.Join(es.Unwrap()...).Error() }

func (es Errors) Unwrap() []error {
	out := make([]error, len(es))
	for i, e := range es {
		out[i] = e
	}
	return out
}

// ErrorHandler renders every error as a JSON:API error document. New always
// installs it; it is exported for apps built without New.
//
// *Error and Errors are rendered as declared. *fiber.Error (404 for unknown
// routes, 405, 413, ...) keeps its status. Anything else becomes a 500 whose
// message is logged, never sent. It assumes no middleware ran, so it is safe
// for requests fasthttp rejects before routing.
func ErrorHandler(c fiber.Ctx, err error) error {
	list := toErrors(err)
	status := documentStatus(list)
	id := requestid.FromContext(c)

	type wire struct {
		ID     string         `json:"id,omitempty"`
		Status string         `json:"status"`
		Code   string         `json:"code,omitempty"`
		Title  string         `json:"title,omitempty"`
		Detail string         `json:"detail,omitempty"`
		Source *ErrorSource   `json:"source,omitempty"`
		Meta   map[string]any `json:"meta,omitempty"`
	}
	out := make([]wire, len(list))
	for i, e := range list {
		out[i] = wire{id, strconv.Itoa(statusOf(e)), e.Code, e.Title, e.Detail, e.Source, e.Meta}
	}

	if status >= fiber.StatusInternalServerError {
		slog.ErrorContext(c.Context(), "http error", "request_id", id, "status", status, "error", err)
	}
	return c.Status(status).JSON(fiber.Map{"errors": out}, MIMEJSONAPI)
}

// toErrors maps any error to the JSON:API objects it is rendered as.
func toErrors(err error) []*Error {
	var es Errors
	if errors.As(err, &es) {
		out := make([]*Error, 0, len(es))
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
	if errors.As(err, &e) {
		return []*Error{e}
	}
	var fe *fiber.Error
	if errors.As(err, &fe) && fe.Code < fiber.StatusInternalServerError {
		detail := fe.Message // Fiber's own messages ("Request Entity Too Large") are safe to show
		if fe.Code == fiber.StatusNotFound {
			detail = "the page you are looking for does not exist"
		}
		return []*Error{{Status: fe.Code, Code: "HTTP-" + strconv.Itoa(fe.Code), Title: http.StatusText(fe.Code), Detail: detail}}
	}
	status := fiber.StatusInternalServerError
	if fe != nil {
		status = fe.Code
	}
	return []*Error{{Status: status, Code: "HTTP-" + strconv.Itoa(status), Title: http.StatusText(status)}}
}

// documentStatus is the status for the whole response: the shared one, else
// the most generally applicable (400 for all-4xx, 500 otherwise), as JSON:API says.
func documentStatus(list []*Error) int {
	s := statusOf(list[0])
	all4xx := true
	for _, e := range list {
		st := statusOf(e)
		all4xx = all4xx && st >= 400 && st < 500
		if st != s {
			s = 0
		}
	}
	switch {
	case s != 0:
		return s
	case all4xx:
		return fiber.StatusBadRequest
	default:
		return fiber.StatusInternalServerError
	}
}

// statusOf treats a missing status as 500: an error nobody classified is ours.
func statusOf(e *Error) int {
	if e == nil || e.Status == 0 {
		return fiber.StatusInternalServerError
	}
	return e.Status
}
