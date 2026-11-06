package xttp

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/gofiber/fiber/v3"
)

var errBadRequest = Error{Status: fiber.StatusBadRequest, Code: "HTTP-400", Title: "Bad Request"}

// Bind decodes the JSON body into in, strictly: unknown fields, trailing data
// and wrong types are a 400 *Error, with Source.Pointer on a type mismatch.
// An empty body leaves in untouched, so check required fields yourself.
func Bind(c fiber.Ctx, in any) error {
	body := c.Body()
	if len(body) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(in); err != nil {
		e := errBadRequest.With("body is not valid for this operation", err)
		var te *json.UnmarshalTypeError
		if errors.As(err, &te) && te.Field != "" {
			e.Source = &ErrorSource{Pointer: "/" + strings.ReplaceAll(te.Field, ".", "/")}
		}
		return e
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errBadRequest.With("body has data after the JSON value", err)
	}
	return nil
}

// BindQuery decodes the query string into q using `query:"name"` tags. A value
// that does not fit its field is a 400 *Error.
func BindQuery(c fiber.Ctx, q any) error {
	if err := c.Bind().Query(q); err != nil {
		return errBadRequest.With("query is not valid for this operation", err)
	}
	return nil
}

// requireQuery answers 400 for each missing name, one error per parameter.
func requireQuery(names []string) fiber.Handler {
	return func(c fiber.Ctx) error {
		var missing Errors
		for _, name := range names {
			if c.Query(name) == "" {
				e := errBadRequest.With("query parameter is required", nil)
				e.Source = &ErrorSource{Parameter: name}
				missing = append(missing, e)
			}
		}
		if len(missing) > 0 {
			return missing
		}
		return c.Next()
	}
}
