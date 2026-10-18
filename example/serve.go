package main

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/moeghifar/libgo/pkg/foreverwork"
	"github.com/moeghifar/libgo/pkg/xttp"
)

// User is the request and the response body: one struct, no wrappers.
type User struct {
	ID    int    `json:"id,omitempty"`
	Name  string `json:"name" doc:"Full name"`
	Email string `json:"email"`
}

// WelcomeJob is queued for every created user.
type WelcomeJob struct{ Email string }

var ErrInvalidUser = xttp.Error{Status: fiber.StatusBadRequest, Code: "USR-400", Title: "Invalid user"}

// serve runs the HTTP API (xttp) and a background worker pool (foreverwork)
// until SIGINT or SIGTERM, then stops both gracefully.
//
//	go run ./example serve
//	curl -X POST localhost:8080/v1/users -d '{"name":"Ada","email":"ada@example.com"}' -H 'Content-Type: application/json'
//	open http://localhost:8080/docs
func serve(ctx context.Context, port int) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wp := foreverwork.New()
	welcome := foreverwork.Register(wp,
		foreverwork.Config{Name: "welcome", Workers: 2, QueueSize: 10, Timeout: 5 * time.Second},
		func(ctx context.Context, j WelcomeJob) error {
			slog.InfoContext(ctx, "welcome email sent", "to", j.Email)
			return nil
		})
	workers := make(chan struct{})
	go func() { wp.Run(ctx); close(workers) }()

	api := xttp.New(xttp.Config{Info: xttp.Info{Title: "Users", Version: "0.1.0"}})
	api.Group("/v1").Add(xttp.Route{
		Method:    fiber.MethodPost,
		Path:      "/users",
		Summary:   "Create a user",
		Tags:      []string{"users"},
		Body:      User{},
		Responses: map[int]any{fiber.StatusCreated: User{}},
		Handler: func(c fiber.Ctx) error {
			var u User
			if err := c.Bind().JSON(&u); err != nil {
				return ErrInvalidUser.With("body must be a JSON user", err)
			}
			if u.Name == "" || u.Email == "" {
				return ErrInvalidUser.With("name and email are required", nil)
			}
			u.ID = 1
			if err := welcome.Submit(c.Context(), WelcomeJob{Email: u.Email}); err != nil {
				return err // pool shutting down: a 500, logged, details hidden
			}
			return c.Status(fiber.StatusCreated).JSON(u)
		},
	})
	api.Docs("/docs")

	err := api.Run(ctx, ":"+strconv.Itoa(port)) // returns after SIGINT/SIGTERM and drain
	cancel()                                    // then stop the workers
	<-workers
	return err
}
