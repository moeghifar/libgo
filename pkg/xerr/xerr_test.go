package xerr

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

var errUserNotFound = Error{Status: 404, Code: "USR-404", Title: "User not found"}

func TestError(t *testing.T) {
	cause := errors.New("sql: no rows")
	tests := []struct {
		name string
		err  *Error
		msg  string
	}{
		{"title", &Error{Title: "oops"}, "oops"},
		{"detail", errUserNotFound.With("no user 42", nil), "User not found: no user 42"},
		{"cause", errUserNotFound.With("no user 42", cause), "User not found: no user 42: sql: no rows"},
		{"cause only", errUserNotFound.With("", cause), "User not found: sql: no rows"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.msg {
				t.Errorf("Error() = %q, want %q", got, tt.msg)
			}
			if tt.err.Err != nil && !errors.Is(tt.err, tt.err.Err) {
				t.Error("Unwrap does not return Err")
			}
		})
	}
	if errUserNotFound.Detail != "" || errUserNotFound.Err != nil {
		t.Fatal("With mutated the shared value")
	}
}

func TestErrorsJoinAndAs(t *testing.T) {
	es := Errors{{Status: 422, Code: "A", Title: "a"}, {Status: 409, Code: "B", Title: "b"}}
	if got := es.Error(); got != "a\nb" {
		t.Errorf("Errors.Error() = %q", got)
	}
	var e *Error
	if !errors.As(fmt.Errorf("w: %w", es), &e) || e.Code != "A" {
		t.Errorf("errors.As through Errors and %%w: %v", e)
	}
}

func TestFlatten(t *testing.T) {
	one := &Error{Status: 404, Code: "ONE"}
	tests := []struct {
		name string
		err  error
		want []string // codes
	}{
		{"nil", nil, nil},
		{"error", one, []string{"ONE"}},
		{"wrapped", fmt.Errorf("w: %w", one), []string{"ONE"}},
		{"errors", Errors{{Code: "A"}, nil, {Code: "B"}}, []string{"A", "B"}},
		{"errors empty", Errors{}, nil},
		{"errors all nil", Errors{nil}, nil},
		{"plain", errors.New("x"), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Flatten(tt.err)
			if len(got) != len(tt.want) {
				t.Fatalf("Flatten = %v, want %v", got, tt.want)
			}
			for i, c := range tt.want {
				if got[i].Code != c {
					t.Errorf("Flatten[%d].Code = %q, want %q", i, got[i].Code, c)
				}
			}
		})
	}
}

func TestStatusOf(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, 0},
		{"error", &Error{Status: 404}, 404},
		{"no status", &Error{Title: "oops"}, 500},
		{"errors shared", Errors{{Status: 422}, {Status: 422}}, 422},
		{"errors all 4xx", Errors{{Status: 422}, {Status: 409}}, 400},
		{"errors mixed", Errors{{Status: 422}, {Status: 503}}, 500},
		{"errors nil member dropped", Errors{{Status: 422}, nil}, 422},
		{"errors empty", Errors{}, 500},
		{"plain", errors.New("x"), 500},
		{"wrapped", fmt.Errorf("w: %w", &Error{Status: 418}), 418},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StatusOf(tt.err); got != tt.want {
				t.Errorf("StatusOf = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCodeOf(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"plain", errors.New("x"), ""},
		{"error", errUserNotFound.With("", nil), "USR-404"},
		{"wrapped", fmt.Errorf("w: %w", errUserNotFound.With("", nil)), "USR-404"},
		{"errors first", Errors{{Code: "A"}, {Code: "B"}}, "A"},
		{"errors nil member", Errors{nil}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CodeOf(tt.err); got != tt.want {
				t.Errorf("CodeOf = %q, want %q", got, tt.want)
			}
			if tt.want != "" && !IsCode(tt.err, tt.want) {
				t.Error("IsCode disagrees with CodeOf")
			}
		})
	}
	if !IsCode(errUserNotFound.With("", nil), "USR-404") || IsCode(errors.New("x"), "USR-404") {
		t.Error("IsCode")
	}
}

// TestStdlibOnly guards the reason this package exists: nothing outside GOROOT
// may be linked by importing it.
func TestStdlibOnly(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	const self = "github.com/moeghifar/libgo/pkg/xerr"
	for _, p := range strings.Fields(string(out)) {
		if p != self && strings.Contains(strings.SplitN(p, "/", 2)[0], ".") {
			t.Errorf("non-stdlib dependency: %s", p)
		}
	}
}
