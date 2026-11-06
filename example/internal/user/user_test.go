package user

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moeghifar/libgo/pkg/xttp"
)

func TestCreate(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		status int
		jobs   int
	}{
		{"valid", `{"name":"Ada","email":"ada@example.com"}`, 201, 1},
		{"missing email", `{"name":"Ada"}`, 400, 0},
		{"not json", `nope`, 400, 0},
		{"unknown field", `{"name":"A","email":"a@b.c","admin":true}`, 400, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobs := 0
			api := xttp.New(xttp.Config{SecuritySchemes: map[string]any{"bearer": map[string]any{"type": "http", "scheme": "bearer"}}})
			api.Group("/v1").Add(Routes(func(context.Context, WelcomeJob) error { jobs++; return nil })...)

			req := httptest.NewRequest("POST", "/v1/users", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := api.App().Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tt.status || jobs != tt.jobs {
				t.Fatalf("got status %d, jobs %d; want %d, %d", resp.StatusCode, jobs, tt.status, tt.jobs)
			}
		})
	}
}
