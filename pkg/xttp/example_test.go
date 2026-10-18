package xttp_test

import (
	"fmt"

	"github.com/gofiber/fiber/v3"

	"github.com/moeghifar/libgo/pkg/xttp"
)

type Pet struct {
	ID   int    `json:"id"`
	Name string `json:"name" doc:"Display name"`
}

func Example() {
	api := xttp.New(xttp.Config{Info: xttp.Info{Title: "Pets", Version: "1.0.0"}})

	api.Group("/v1").Add(xttp.Route{
		Method:    fiber.MethodGet,
		Path:      "/pets/:id",
		Summary:   "Get a pet",
		Responses: map[int]any{200: Pet{}},
		Handler: func(c fiber.Ctx) error {
			return c.JSON(Pet{ID: 1, Name: "Rex"})
		},
	})
	api.Docs("/docs")

	fmt.Print(string(api.YAML()))
	// Output:
	// components:
	//   schemas:
	//     Pet:
	//       properties:
	//         id:
	//           type: "integer"
	//         name:
	//           description: "Display name"
	//           type: "string"
	//       required:
	//         - "id"
	//         - "name"
	//       type: "object"
	// info:
	//   title: "Pets"
	//   version: "1.0.0"
	// openapi: "3.1.0"
	// paths:
	//   "/v1/pets/{id}":
	//     get:
	//       parameters:
	//         -
	//           in: "path"
	//           name: "id"
	//           required: true
	//           schema:
	//             type: "string"
	//       responses:
	//         "200":
	//           content:
	//             application/json:
	//               schema:
	//                 $ref: "#/components/schemas/Pet"
	//           description: "OK"
	//       summary: "Get a pet"
}
