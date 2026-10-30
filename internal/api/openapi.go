package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strconv"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
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

// specAPI is a huma API backed by a private router; it renders the spec and
// the docs UI, and never serves the real endpoints.
type specAPI struct {
	api huma.API
	mux *chi.Mux
}

func newSpecAPI() specAPI {
	mux := chi.NewMux()
	cfg := huma.DefaultConfig("Speedtest Tracker API", "1.0.0")
	cfg.Info.Description = "REST API of speedtest-tracker. Authenticate with an API token " +
		"(`Authorization: Bearer <token>`), the OIDC session cookie, or the " +
		"reverse proxy's forward-auth headers, depending on the configured auth mode."
	cfg.CreateHooks = nil
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearer": {Type: "http", Scheme: "bearer", Description: "API token created under Settings > Tokens (auth mode token)."},
		"cookie": {Type: "apiKey", In: "cookie", Name: "st_session", Description: "Session cookie set by the OIDC login flow."},
		"forwardAuth": {Type: "apiKey", In: "header", Name: "Remote-User",
			Description: "Identity header injected by a trusted reverse proxy (forward_auth mode); the header name is configurable."},
	}
	cfg.Security = []map[string][]string{{"bearer": {}}, {"cookie": {}}, {"forwardAuth": {}}}
	cfg.OpenAPIPath = "/openapi"
	cfg.DocsPath = "/docs"
	a := humachi.New(mux, cfg)
	oapi := a.OpenAPI()
	reg := oapi.Components.Schemas
	var ops []opSpec
	for _, g := range specGroups {
		ops = append(ops, g()...)
	}
	for _, s := range ops {
		oapi.AddOperation(s.operation(reg))
	}
	return specAPI{api: a, mux: mux}
}

// OpenAPIJSON renders the API description as indented JSON.
func OpenAPIJSON() ([]byte, error) {
	b, err := json.MarshalIndent(newSpecAPI().api.OpenAPI(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// mountDocs serves /openapi.{json,yaml} and /docs behind authMW.
func (s specAPI) mountDocs(r chi.Router, authMW func(http.Handler) http.Handler) {
	for _, path := range []string{"/openapi.json", "/openapi.yaml", "/openapi-3.0.json", "/openapi-3.0.yaml", "/docs"} {
		r.With(authMW).Get(path, s.mux.ServeHTTP)
	}
}
