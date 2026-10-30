package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
)

// The handlers stay plain chi so request validation and the error envelope
// remain byte-for-byte what they were. Huma is used purely as the OpenAPI
// 3.1 document builder and docs renderer: every route is described by an
// opSpec in the spec_*.go files, and TestSpecCoversRoutes fails when a
// registered /api/v1 route has no matching operation.

// p describes one path or query parameter.
type p struct {
	Name, Doc, Type string // Type: integer (default) or string
	Query           bool
	Required        bool
	Enum            []any
}

// opSpec documents one HTTP operation.
type opSpec struct {
	Method, Path, ID, Tag, Summary string
	Params                         []p
	Body                           any // request body type (JSON), nil for none
	OptionalBody                   bool
	Status                         int    // success status (default 200, or 204 without Out)
	Out                            any    // response body type, nil for no body
	OutType                        string // response media type override
	Errors                         []int  // extra error statuses
	NoAuth                         bool
}

func init() {
	// Handlers return empty collections as [] (or the frontend guards for
	// it); the schema should not force every consumer to handle null.
	huma.DefaultArrayNullable = false
}

var specGroups []func() []opSpec

func registerSpec(f func() []opSpec) { specGroups = append(specGroups, f) }

func idParam(doc string) p { return p{Name: "id", Doc: doc, Required: true} }

func ptr[T any](v T) *T { return &v }

func (s opSpec) operation(reg huma.Registry) *huma.Operation {
	op := &huma.Operation{
		OperationID: s.ID,
		Method:      s.Method,
		Path:        s.Path,
		Summary:     s.Summary,
		Tags:        []string{s.Tag},
		Responses:   map[string]*huma.Response{},
	}
	for _, q := range s.Params {
		typ := q.Type
		if typ == "" {
			typ = "integer"
		}
		in := "path"
		if q.Query {
			in = "query"
		}
		prm := &huma.Param{Name: q.Name, In: in, Description: q.Doc,
			Required: q.Required || !q.Query,
			Schema:   &huma.Schema{Type: typ, Enum: q.Enum}}
		op.Parameters = append(op.Parameters, prm)
	}
	if s.Body != nil {
		op.RequestBody = &huma.RequestBody{
			Required: !s.OptionalBody,
			Content: map[string]*huma.MediaType{"application/json": {
				Schema: reg.Schema(reflect.TypeOf(s.Body), true, ""),
			}},
		}
	}
	status := s.Status
	if status == 0 {
		status = http.StatusOK
		if s.Out == nil {
			status = http.StatusNoContent
		}
	}
	ok := &huma.Response{Description: http.StatusText(status)}
	if s.Out != nil {
		mt := s.OutType
		if mt == "" {
			mt = "application/json"
		}
		ok.Content = map[string]*huma.MediaType{mt: {
			Schema: reg.Schema(reflect.TypeOf(s.Out), true, ""),
		}}
	}
	op.Responses[strconv.Itoa(status)] = ok
	errs := map[int]bool{}
	if !s.NoAuth {
		errs[http.StatusUnauthorized] = true
	}
	if s.Method != http.MethodGet && !s.NoAuth {
		errs[http.StatusForbidden] = true
	}
	if s.Body != nil || len(s.Params) > 0 {
		errs[http.StatusBadRequest] = true
	}
	for _, q := range s.Params {
		if !q.Query {
			errs[http.StatusNotFound] = true
		}
	}
	for _, e := range s.Errors {
		errs[e] = true
	}
	errSchema := reg.Schema(reflect.TypeOf(ErrorResponse{}), true, "")
	codes := make([]int, 0, len(errs))
	for c := range errs {
		codes = append(codes, c)
	}
	sort.Ints(codes)
	for _, c := range codes {
		op.Responses[strconv.Itoa(c)] = &huma.Response{
			Description: http.StatusText(c),
			Content:     map[string]*huma.MediaType{"application/json": {Schema: errSchema}},
		}
	}
	if s.NoAuth {
		op.Security = []map[string][]string{}
	}
	return op
}

// addDocsOnly documents the routes that stay on hand-written chi handlers
// because they are not JSON request/response (stream, download, redirects).
func addDocsOnly(api huma.API) {
	oapi := api.OpenAPI()
	reg := oapi.Components.Schemas
	for _, g := range specGroups {
		for _, s := range g() {
			oapi.AddOperation(s.operation(reg))
		}
	}
}

// OpenAPIJSON renders the API description as indented JSON. It registers
// every typed operation regardless of which optional dependencies a
// running server would have.
func OpenAPIJSON() ([]byte, error) {
	api := newHumaAPI(chi.NewMux())
	Deps{}.registerAPI(api, true)
	addDocsOnly(api)
	b, err := json.MarshalIndent(api.OpenAPI(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// etagPaths are the list endpoints that get a strong ETag / 304 handling.
var etagPaths = map[string]bool{
	"/api/v1/targets": true, "/api/v1/queues": true, "/api/v1/schedules": true,
	"/api/v1/runs": true, "/api/v1/results": true,
}

// etagMiddleware applies etagJSON's behaviour to the list endpoints.
func etagMiddleware(next http.Handler) http.Handler {
	wrapped := etagJSON(next.ServeHTTP)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && etagPaths[strings.TrimSuffix(r.URL.Path, "/")] {
			wrapped(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// registerAPI registers every typed operation. all forces the ones that
// depend on optional dependencies (used to render the full document).
func (d Deps) registerAPI(api huma.API, all bool) {
	register(api, route{Method: http.MethodGet, Path: "/api/v1/me", ID: "getMe", Tag: "me", Summary: "Resolved identity of the caller"}, d.me)
	if !all && d.Store == nil {
		return
	}
	d.registerTargets(api)
	d.registerQueues(api)
	d.registerSchedules(api)
	d.registerRuns(api)
	d.registerResults(api)
	//@groups
}

// Shared parameter docs for the groups still described by opSpec.
var (
	rangeParam  = p{Name: "range", Query: true, Type: "string", Doc: "Named window ending now.", Enum: []any{"24h", "7d", "30d"}}
	offsetParam = p{Name: "offset", Query: true, Doc: "1 shifts the window back by its own span (previous period); 0 or omitted keeps it.", Enum: []any{0, 1}}
	limitParam  = p{Name: "limit", Query: true, Doc: "Maximum rows to return."}
	cursorParam = p{Name: "cursor", Query: true, Type: "string", Doc: "Opaque cursor from a previous page's next_cursor."}
)
