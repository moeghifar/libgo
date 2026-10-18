// Package xttp registers Fiber v3 routes that also describe themselves as an
// OpenAPI 3.1 document.
//
// It is not a framework over Fiber. Handlers stay plain fiber.Handler: you read
// the request and write the response yourself, with all of Fiber's control.
// The types on a Route — Query, Body, Responses — exist only to describe the
// operation. They are reflected once, when the route is added, and never touched
// on a request.
//
//	api := xttp.New(xttp.Config{Info: xttp.Info{Title: "Users", Version: "1.0.0"}})
//	api.Add(xttp.Route{
//		Method:    fiber.MethodGet,
//		Path:      "/users/:id",
//		Summary:   "Get a user",
//		Responses: map[int]any{200: User{}, 404: Error{}},
//		Handler:   getUser,
//	})
//	api.Docs("/docs") // /docs, /docs/openapi.yaml, /docs/openapi.json
//	api.Run(context.Background(), ":3000") // until SIGINT/SIGTERM
package xttp

import (
	"encoding/json"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
)

// Info is the document's info object.
type Info struct {
	Title       string
	Version     string
	Description string
}

// Route is one operation: how Fiber serves it and how the document describes it.
type Route struct {
	Method      string // fiber.MethodGet, ...
	Path        string // Fiber syntax: "/users/:id"; ":id?" marks an optional param
	Summary     string
	Description string
	OperationID string
	Tags        []string
	Deprecated  bool

	// Query is a struct whose `query:"name"` fields become query parameters.
	// A `required:"true"` tag marks one required; `doc:"..."` describes it.
	Query any
	// Body is the JSON request body type, e.g. CreateUser{}. Nil means none.
	Body any
	// Responses maps a status code to its JSON body type. A nil value
	// documents the status with no body.
	Responses map[int]any

	Handler     fiber.Handler
	Middlewares []fiber.Handler // run before Handler, on this route only
}

// API registers routes on a Fiber router and collects their description.
// Group returns an API that shares the same document.
type API struct {
	app    *fiber.App
	listen fiber.ListenConfig
	router fiber.Router
	prefix string
	doc    *document
}

type document struct {
	mu      sync.Mutex
	info    Info
	paths   map[string]any
	schemas map[string]any
	names   map[reflect.Type]string
}

// Group returns an API whose routes are mounted under prefix, both in Fiber
// and in the document. Middlewares apply to every route in the group.
func (a *API) Group(prefix string, middlewares ...fiber.Handler) *API {
	var g fiber.Router
	if len(middlewares) > 0 {
		args := make([]any, len(middlewares))
		for i, m := range middlewares {
			args[i] = m
		}
		g = a.router.Group(prefix, args...)
	} else {
		g = a.router.Group(prefix)
	}
	return &API{app: a.app, listen: a.listen, router: g, prefix: a.prefix + prefix, doc: a.doc}
}

// Add registers routes with Fiber and in the document. It panics on a route
// with no Method, Path or Handler: that is a programmer error caught at startup.
func (a *API) Add(routes ...Route) {
	for _, r := range routes {
		if r.Method == "" || r.Path == "" || r.Handler == nil {
			panic("xttp: route needs Method, Path and Handler: " + r.Method + " " + r.Path)
		}
		// Fiber runs the chain in argument order, so middlewares go first.
		chain := make([]any, 0, len(r.Middlewares)+1)
		for _, m := range r.Middlewares {
			chain = append(chain, m)
		}
		chain = append(chain, r.Handler)
		a.router.Add([]string{r.Method}, r.Path, chain[0], chain[1:]...)
		a.doc.add(a.prefix, r)
	}
}

// Spec returns the document as plain maps, ready for json or yaml encoding.
func (a *API) Spec() map[string]any {
	d := a.doc
	d.mu.Lock()
	defer d.mu.Unlock()
	info := map[string]any{"title": d.info.Title, "version": d.info.Version}
	if d.info.Description != "" {
		info["description"] = d.info.Description
	}
	spec := map[string]any{"openapi": "3.1.0", "info": info, "paths": d.paths}
	if len(d.schemas) > 0 {
		spec["components"] = map[string]any{"schemas": d.schemas}
	}
	return spec
}

// YAML returns the document as YAML.
func (a *API) YAML() []byte { return marshalYAML(a.Spec()) }

// JSON returns the document as indented JSON.
func (a *API) JSON() []byte {
	b, _ := json.MarshalIndent(a.Spec(), "", "  ") // plain maps of scalars: cannot fail
	return b
}

// Docs serves the document and a browsable reference under path:
// path (Scalar viewer), path+"/openapi.yaml" and path+"/openapi.json".
// The document is rendered on the first request, so add routes before serving.
func (a *API) Docs(path string) {
	yamlDoc := sync.OnceValue(a.YAML)
	jsonDoc := sync.OnceValue(a.JSON)
	page := docsPage(a.doc.info.Title, a.prefix+path+"/openapi.json")
	a.router.Get(path+"/openapi.yaml", func(c fiber.Ctx) error {
		c.Set(fiber.HeaderContentType, "application/yaml")
		return c.Send(yamlDoc())
	})
	a.router.Get(path+"/openapi.json", func(c fiber.Ctx) error {
		c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		return c.Send(jsonDoc())
	})
	a.router.Get(path, func(c fiber.Ctx) error {
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return c.SendString(page)
	})
}

var pathParam = regexp.MustCompile(`:([A-Za-z0-9_]+)(\?)?`)

func (d *document) add(prefix string, r Route) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var params []any
	for _, m := range pathParam.FindAllStringSubmatch(r.Path, -1) {
		params = append(params, map[string]any{
			"name": m[1], "in": "path", "required": true, "schema": map[string]any{"type": "string"},
		})
	}
	path := pathParam.ReplaceAllString(prefix+r.Path, "{$1}")

	op := map[string]any{}
	set := func(k string, v any, ok bool) {
		if ok {
			op[k] = v
		}
	}
	set("summary", r.Summary, r.Summary != "")
	set("description", r.Description, r.Description != "")
	set("operationId", r.OperationID, r.OperationID != "")
	set("tags", toAny(r.Tags), len(r.Tags) > 0)
	set("deprecated", true, r.Deprecated)

	if r.Query != nil {
		params = append(params, d.queryParams(reflect.TypeOf(r.Query))...)
	}
	set("parameters", params, len(params) > 0)

	if r.Body != nil {
		op["requestBody"] = map[string]any{"required": true, "content": jsonContent(d.schema(reflect.TypeOf(r.Body)))}
	}

	responses := map[string]any{}
	for code, body := range r.Responses {
		resp := map[string]any{"description": http.StatusText(code)}
		if body != nil {
			resp["content"] = jsonContent(d.schema(reflect.TypeOf(body)))
		}
		responses[strconv.Itoa(code)] = resp
	}
	if len(responses) == 0 {
		responses["default"] = map[string]any{"description": "Response"}
	}
	op["responses"] = responses

	item, _ := d.paths[path].(map[string]any)
	if item == nil {
		item = map[string]any{}
		d.paths[path] = item
	}
	item[strings.ToLower(r.Method)] = op
}

func (d *document) queryParams(t reflect.Type) []any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	var out []any
	for i := range t.NumField() {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("query"), ",")
		if name == "" || name == "-" {
			continue
		}
		p := map[string]any{"name": name, "in": "query", "schema": d.schema(f.Type)}
		if f.Tag.Get("required") == "true" {
			p["required"] = true
		}
		if doc := f.Tag.Get("doc"); doc != "" {
			p["description"] = doc
		}
		out = append(out, p)
	}
	return out
}

var timeType = reflect.TypeFor[time.Time]()

// schema reflects t into a JSON Schema. Named structs go to components and are
// returned as a $ref, which is also what stops recursive types from looping.
func (d *document) schema(t reflect.Type) map[string]any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch {
	case t == timeType:
		return map[string]any{"type": "string", "format": "date-time"}
	case t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8:
		return map[string]any{"type": "string", "format": "byte"}
	}
	switch t.Kind() {
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Slice, reflect.Array:
		return map[string]any{"type": "array", "items": d.schema(t.Elem())}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": d.schema(t.Elem())}
	case reflect.Struct:
		if t.Name() == "" {
			return d.object(t)
		}
		name, ok := d.names[t]
		if !ok {
			name = d.nameFor(t)
			d.names[t] = name
			d.schemas[name] = map[string]any{} // placeholder: a self-reference finds the name
			d.schemas[name] = d.object(t)
		}
		return map[string]any{"$ref": "#/components/schemas/" + name}
	}
	return map[string]any{} // interface, func, chan: any value
}

var nonIdent = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func (d *document) nameFor(t reflect.Type) string {
	name := nonIdent.ReplaceAllString(t.Name(), "_")
	if _, taken := d.schemas[name]; taken {
		pkg := t.PkgPath()
		name = nonIdent.ReplaceAllString(pkg[strings.LastIndex(pkg, "/")+1:], "_") + "." + name
	}
	return name
}

func (d *document) object(t reflect.Type) map[string]any {
	props := map[string]any{}
	var required []any
	d.fields(t, props, &required)
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

// fields follows encoding/json: unexported fields and `json:"-"` are skipped,
// untagged embedded structs are flattened, and a field without omitempty or
// omitzero is required.
func (d *document) fields(t reflect.Type, props map[string]any, required *[]any) {
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if f.Anonymous && name == "" {
			et := f.Type
			if et.Kind() == reflect.Pointer {
				et = et.Elem()
			}
			if et.Kind() == reflect.Struct {
				d.fields(et, props, required)
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		s := d.schema(f.Type)
		if doc := f.Tag.Get("doc"); doc != "" {
			if _, isRef := s["$ref"]; isRef {
				s = map[string]any{"allOf": []any{s}, "description": doc}
			} else {
				s["description"] = doc
			}
		}
		props[name] = s
		if !strings.Contains(opts, "omitempty") && !strings.Contains(opts, "omitzero") {
			*required = append(*required, name)
		}
	}
}

func jsonContent(schema map[string]any) map[string]any {
	return map[string]any{fiber.MIMEApplicationJSON: map[string]any{"schema": schema}}
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

const scalarJS = "https://cdn.jsdelivr.net/npm/@scalar/api-reference@1.25.122/dist/browser/standalone.min.js"

func docsPage(title, specURL string) string {
	return `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>` +
		escapeHTML(title) + `</title></head>
<body>
<script id="api-reference" data-url="` + escapeHTML(specURL) + `"></script>
<script src="` + scalarJS + `"></script>
</body>
</html>`
}

var escapeHTML = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace
