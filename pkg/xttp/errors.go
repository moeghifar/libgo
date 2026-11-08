package xttp

import (
	"cmp"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"

	"github.com/moeghifar/libgo/pkg/xerr"
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
// It is xerr.Error: code that does not serve HTTP imports pkg/xerr instead
// and returns the same values, without linking Fiber.
type Error = xerr.Error

// ErrorSource points at the part of the request that caused an error. Set one field. It is xerr.ErrorSource.
type ErrorSource = xerr.ErrorSource

// Errors reports several problems at once, e.g. one per invalid field. It is xerr.Errors.
type Errors = xerr.Errors

// ErrorDocument is the body of every error response: a JSON:API error
// document. Use it as a Responses value to document an error status, or let
// Config.DefaultErrors add it everywhere.
type ErrorDocument struct {
	Errors []ErrorObject `json:"errors"`
}

// ErrorObject is one error as sent on the wire; Error is the Go-side value it
// is rendered from.
type ErrorObject struct {
	ID     string         `json:"id,omitempty" doc:"Request id"`
	Status string         `json:"status" doc:"HTTP status, as a string"`
	Code   string         `json:"code,omitempty" doc:"Application-specific code"`
	Title  string         `json:"title,omitempty"`
	Detail string         `json:"detail,omitempty"`
	Source *ErrorSource   `json:"source,omitempty"`
	Meta   map[string]any `json:"meta,omitempty"`
}

// StatusOf is the HTTP status ErrorHandler sends for err: 0 for nil,
// otherwise the declared status of *Error, the document status of Errors
// (shared, else 400 for all-4xx, else 500), a *fiber.Error's code, or 500.
// Middleware that reads the status after c.Next() needs it, because the
// error handler sets the response status only after the chain returned.
func StatusOf(err error) int {
	if err == nil {
		return 0
	}
	return toErrors(err).Status()
}

// ErrorHandler renders every error as a JSON:API error document. New installs
// the (*API).ErrorHandler method, which also adds the configured security
// headers and logs through Config.Logger; this free function keeps working
// for apps built without New, with the default logger.
//
// *Error and Errors are rendered as declared. *fiber.Error (404 for unknown
// routes, 405, 413, ...) keeps its status. Anything else becomes a 500 whose
// message is logged, never sent. It assumes no middleware ran, so it is safe
// for requests fasthttp rejects before routing.
func ErrorHandler(c fiber.Ctx, err error) error {
	return renderError(c, err, slog.Default())
}

// ErrorHandler is the error handler New installs: ErrorHandler plus the
// resolved security headers (so 413/431 responses carry them) and Config.Logger.
func (a *API) ErrorHandler(c fiber.Ctx, err error) error {
	a.headers.set(c)
	return renderError(c, a.mapped(c, err), a.log)
}

// mapped applies Config.MapError to an error nothing has classified yet.
func (a *API) mapped(c fiber.Ctx, err error) (out error) {
	out = err
	var (
		e  *Error
		es Errors
		fe *fiber.Error
	)
	if a.mapError == nil || err == nil || errors.As(err, &e) || errors.As(err, &es) || errors.As(err, &fe) {
		return out
	}
	defer a.contain(c, "MapError")
	if m := a.mapError(err); m != nil {
		out = m
	}
	return out
}

// contain logs and drops a panic from a user hook: these run outside the
// recover middleware, where a panic would take the process down.
func (a *API) contain(c fiber.Ctx, hook string) {
	if r := recover(); r != nil {
		a.log.ErrorContext(c.Context(), "xttp: hook panicked", "hook", hook, "panic", r)
	}
}

// CodeOf returns the Code of the first *Error in err's chain, or "". It is xerr.CodeOf.
func CodeOf(err error) string { return xerr.CodeOf(err) }

// IsCode reports whether CodeOf(err) is code. It is xerr.IsCode.
func IsCode(err error, code string) bool { return xerr.IsCode(err, code) }

func renderError(c fiber.Ctx, err error, log *slog.Logger) error {
	list := toErrors(err)
	status := list.Status()
	id := requestid.FromContext(c)

	doc := ErrorDocument{Errors: make([]ErrorObject, len(list))}
	for i, e := range list {
		doc.Errors[i] = ErrorObject{id, strconv.Itoa(cmp.Or(e.Status, fiber.StatusInternalServerError)), e.Code, e.Title, e.Detail, e.Source, e.Meta}
	}

	if status >= fiber.StatusInternalServerError {
		log.ErrorContext(c.Context(), "http error", "request_id", id, "status", status, "error", err)
	}
	return c.Status(status).JSON(doc, MIMEJSONAPI)
}

// toErrors maps any error to the JSON:API objects it is rendered as: the
// *Error values it carries (xerr.Flatten), else a *fiber.Error's status, else 500.
func toErrors(err error) Errors {
	if list := xerr.Flatten(err); len(list) > 0 {
		return list
	}
	var fe *fiber.Error
	if errors.As(err, &fe) && fe.Code < fiber.StatusInternalServerError {
		detail := fe.Message // Fiber's own messages ("Request Entity Too Large") are safe to show
		if fe.Code == fiber.StatusNotFound {
			detail = "the page you are looking for does not exist"
		}
		return Errors{{Status: fe.Code, Code: "HTTP-" + strconv.Itoa(fe.Code), Title: http.StatusText(fe.Code), Detail: detail}}
	}
	status := fiber.StatusInternalServerError
	if fe != nil {
		status = fe.Code
	}
	return Errors{{Status: status, Code: "HTTP-" + strconv.Itoa(status), Title: http.StatusText(status)}}
}
