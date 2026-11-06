// Package user is a sample business module: its types, routes and jobs live
// together, and it depends on the worker pool only through a func.
package user

import (
	"context"
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/moeghifar/libgo/pkg/xttp"
)

// User is the request and the response body.
type User struct {
	ID    int    `json:"id,omitempty"`
	Name  string `json:"name" doc:"Full name"`
	Email string `json:"email"`
}

// WelcomeJob is queued for every created user.
type WelcomeJob struct{ Email string }

// ErrInvalid is returned for a body that is not a valid user.
var ErrInvalid = xttp.Error{Status: fiber.StatusBadRequest, Code: "USR-400", Title: "Invalid user"}

// SendWelcome handles a WelcomeJob.
func SendWelcome(ctx context.Context, _ WelcomeJob) error {
	slog.InfoContext(ctx, "welcome email sent")
	return nil
}

// Routes returns the module's routes. submit queues a welcome job.
func Routes(submit func(context.Context, WelcomeJob) error) []xttp.Route {
	return []xttp.Route{{
		Method:    fiber.MethodPost,
		Path:      "/users",
		Summary:   "Create a user",
		Tags:      []string{"users"},
		Body:      User{},
		Security:  []string{"bearer"},
		Responses: map[int]any{fiber.StatusCreated: User{}},
		Handler: func(c fiber.Ctx) error {
			var u User
			if err := xttp.Bind(c, &u); err != nil {
				return err
			}
			if u.Name == "" || u.Email == "" {
				return ErrInvalid.With("name and email are required", nil)
			}
			u.ID = 1
			if err := submit(c.Context(), WelcomeJob{Email: u.Email}); err != nil {
				return err // pool shutting down: a 500, logged, details hidden
			}
			return c.Status(fiber.StatusCreated).JSON(u)
		},
	}}
}
