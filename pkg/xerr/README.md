# xerr

`xerr` is the typed error model shared by libgo packages: a [JSON:API error object](https://jsonapi.org/format/#error-objects) with an HTTP status, an application code and a hidden internal cause. It imports only the standard library, so workers, CLIs and gRPC servers return the same errors that `xttp` renders, without linking an HTTP stack. `xttp.Error`, `xttp.Errors` and `xttp.ErrorSource` are aliases of these types.

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

## License

Apache 2.0, see [LICENSE](../../LICENSE)
