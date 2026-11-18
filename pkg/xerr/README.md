# xerr

One typed error model for HTTP, gRPC, workers and CLIs.

An `xerr.Error` is a [JSON:API error object](https://jsonapi.org/format/#error-objects) with an HTTP status, an application code and a hidden internal cause. The package imports only the standard library, so workers, CLIs and gRPC servers can return the same errors that `xttp` renders, without linking an HTTP stack. `xttp.Error`, `xttp.Errors` and `xttp.ErrorSource` are aliases of these types.

```sh
go get github.com/moeghifar/libgo/pkg/xerr
```

## Features

- Declare once, return a copy per occurrence: `With(detail, cause)` never mutates the shared value
- Only `*Error` is an `error`, so a shared value can't be returned, or mutated across requests, by accident
- `Err` is the internal cause: `errors.Is`/`errors.As` and logs see it, clients never do
- `Errors` reports several problems at once (one per invalid field); its status is the shared one, else 400 for all-4xx, else 500
- `StatusOf`, `CodeOf`, `IsCode` read status and code back through `fmt.Errorf("%w")` wrapping
- `Flatten` extracts the `*Error` values an error stands for, for renderers
- Standard library only, verified by a test

## Usage

```go
package user

import (
	"errors"
	"fmt"

	"github.com/moeghifar/libgo/pkg/xerr"
)

var (
	ErrNotFound = xerr.Error{Status: 404, Code: "USR-404", Title: "User not found"}
	ErrInvalid  = xerr.Error{Status: 422, Code: "USR-422", Title: "Invalid user"}
)

func (s *Service) Find(id string) (*User, error) {
	u, err := s.repo.Find(id)
	if err != nil {
		return nil, ErrNotFound.With("no user with id "+id, err) // err is logged, never sent
	}
	return u, nil
}

func (s *Service) Validate(u User) error {
	var errs xerr.Errors
	if u.Email == "" {
		e := ErrInvalid.With("email is required", nil)
		e.Source = &xerr.ErrorSource{Pointer: "/email"}
		errs = append(errs, e)
	}
	if u.Name == "" {
		errs = append(errs, ErrInvalid.With("name is required", nil))
	}
	if len(errs) > 0 {
		return errs // status 422: shared by all members
	}
	return nil
}

// Anywhere, HTTP or not:
func report(err error) {
	fmt.Println(xerr.StatusOf(err), xerr.CodeOf(err)) // 404 USR-404
	if xerr.IsCode(err, "USR-404") { /* ... */ }
	var e *xerr.Error
	if errors.As(err, &e) { /* e.Title, e.Detail, e.Err */ }
}
```

In an `xttp` handler the same error renders as:

```json
{"errors":[{"id":"<request id>","status":"404","code":"USR-404","title":"User not found","detail":"no user with id 42"}]}
```

## Reference

| Field of `Error` | Meaning |
| --- | --- |
| `Status` | HTTP status |
| `Code` | Application code, such as `USR-404` |
| `Title` | Short summary, the same for every occurrence |
| `Detail` | This occurrence, shown to the client |
| `Source` | `*ErrorSource{Pointer, Parameter, Header}`: the part of the request at fault. Set one field. |
| `Meta` | Extra non-standard data |
| `Err` | Internal cause: logged and visible to `errors.Is`/`errors.As`, never sent |

| Function | What it does |
| --- | --- |
| `(Error).With(detail, err) *Error` | Copy with this occurrence's detail and cause |
| `StatusOf(err) int` | Status the error stands for: 0 for nil, 500 for a plain error |
| `CodeOf(err) string` | Code of the first `*Error` found, `""` if none |
| `IsCode(err, code) bool` | Whether `CodeOf(err)` equals `code` |
| `Flatten(err) Errors` | The `*Error` values an error stands for, for renderers |

## Not included

- No rendering. `xerr` has no HTTP or gRPC code; `xttp` renders the JSON:API document, and a gRPC server maps `StatusOf` to a status code.
- No stack traces and no error catalogue. Declare your errors as package-level values next to the code that returns them.

## License

Apache 2.0, see [LICENSE](../../LICENSE)
