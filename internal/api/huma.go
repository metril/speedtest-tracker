package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"
	"sort"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// Typed handlers are registered through huma, which derives the OpenAPI
// document from their Input/Output structs. Huma's own validation is
// switched off (SkipValidateParams/SkipValidateBody) and replaced by the
// handlers' hand-written checks, so status codes, messages and the
// {error:{code,message}} envelope stay exactly what the chi handlers
// produced.

func init() {
	huma.NewError = newHumaError
	// Handlers return empty collections as [] (or the frontend guards for
	// it); the schema should not force every consumer to handle null.
	huma.DefaultArrayNullable = false
}

// APIError is the error every typed handler returns: the legacy
// {error:{code,message}} envelope plus the HTTP status.
type APIError struct {
	ErrorResponse
	status int
}

// GetStatus implements huma.StatusError.
func (e *APIError) GetStatus() int { return e.status }

// Error implements error.
func (e *APIError) Error() string { return e.ErrorResponse.Error.Message }

// ContentType keeps errors as plain application/json (huma defaults to
// application/problem+json).
func (e *APIError) ContentType(string) string { return "application/json" }

// apiErr builds a typed error response.
func apiErr(status int, code, message string) *APIError {
	e := &APIError{status: status}
	e.ErrorResponse.Error.Code = code
	e.ErrorResponse.Error.Message = message
	return e
}

func badRequest(msg string) error     { return apiErr(http.StatusBadRequest, "invalid_request", msg) }
func notFound(msg string) error       { return apiErr(http.StatusNotFound, "not_found", msg) }
func forbidden(msg string) error      { return apiErr(http.StatusForbidden, "forbidden", msg) }
func conflict(code, msg string) error { return apiErr(http.StatusConflict, code, msg) }

// newHumaError maps huma's own failures (unparseable path parameter,
// malformed body) onto the legacy envelope.
func newHumaError(status int, msg string, errs ...error) huma.StatusError {
	message := msg
	for _, e := range errs {
		var d *huma.ErrorDetail
		if !errors.As(e, &d) {
			continue
		}
		switch {
		case d.Location == "body":
			message = d.Message
		case strings.HasPrefix(d.Location, "path.id"):
			message = "id must be a positive integer"
		case strings.HasPrefix(d.Location, "path.version"):
			message = "version must be a positive integer"
		default:
			message = d.Location + ": " + d.Message
		}
		break
	}
	if msg == "request body is required" {
		message = "malformed JSON body: EOF"
	}
	code := "invalid_request"
	if strings.Contains(message, "unknown content type") {
		return apiErr(http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json")
	}
	switch status {
	case http.StatusRequestEntityTooLarge:
		status = http.StatusBadRequest
		message = "malformed JSON body: http: request body too large"
	case http.StatusUnprocessableEntity:
		status = http.StatusBadRequest
	case http.StatusUnsupportedMediaType:
		code = "unsupported_media_type"
	case http.StatusNotFound:
		code = "not_found"
	case http.StatusUnauthorized:
		code = "unauthorized"
	case http.StatusForbidden:
		code = "forbidden"
	case http.StatusMethodNotAllowed:
		code = "method_not_allowed"
	}
	return apiErr(status, code, message)
}

// errCapture lets the legacy validators, which report problems by writing
// to an http.ResponseWriter, feed a typed handler's error return.
type errCapture struct {
	h      http.Header
	status int
	buf    bytes.Buffer
}

func (c *errCapture) Header() http.Header {
	if c.h == nil {
		c.h = http.Header{}
	}
	return c.h
}
func (c *errCapture) WriteHeader(s int) {
	if c.status == 0 {
		c.status = s
	}
}
func (c *errCapture) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	return c.buf.Write(b)
}

// err converts the captured response into a typed error.
func (c *errCapture) err() error {
	var e ErrorResponse
	if json.Unmarshal(c.buf.Bytes(), &e) != nil || c.status < 400 {
		return apiErr(http.StatusInternalServerError, "internal_error", "internal error")
	}
	return &APIError{ErrorResponse: e, status: c.status}
}

// jsonFormat encodes like writeJSON did (HTML escaping on) and decodes
// like decodeJSON did (unknown fields rejected).
var jsonFormat = huma.Format{
	Marshal: func(w io.Writer, v any) error {
		return json.NewEncoder(w).Encode(v)
	},
	Unmarshal: func(data []byte, v any) error {
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(v); err != nil {
			return errors.New("malformed JSON body: " + err.Error())
		}
		return nil
	},
}

// newHumaAPI builds the huma API on r. The docs and spec routes it adds
// inherit whatever middleware r carries.
func newHumaAPI(r chi.Router) huma.API {
	cfg := huma.DefaultConfig("Speedtest Tracker API", "1.0.0")
	cfg.Info.Description = "REST API of speedtest-tracker. Authenticate with an API token " +
		"(`Authorization: Bearer <token>`), the OIDC session cookie, or the " +
		"reverse proxy's forward-auth headers, depending on the configured auth mode."
	cfg.CreateHooks = nil
	cfg.Formats = map[string]huma.Format{"application/json": jsonFormat, "json": jsonFormat}
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearer": {Type: "http", Scheme: "bearer", Description: "API token created under Settings > Tokens (auth mode token)."},
		"cookie": {Type: "apiKey", In: "cookie", Name: "st_session", Description: "Session cookie set by the OIDC login flow."},
		"forwardAuth": {Type: "apiKey", In: "header", Name: "Remote-User",
			Description: "Identity header injected by a trusted reverse proxy (forward_auth mode); the header name is configurable."},
	}
	cfg.Security = []map[string][]string{{"bearer": {}}, {"cookie": {}}, {"forwardAuth": {}}}
	cfg.OpenAPIPath = "/openapi"
	cfg.DocsPath = "/docs"
	return humachi.New(r, cfg)
}

// route describes one typed operation.
type route struct {
	Method, Path, ID, Tag, Summary string
	Status                         int   // success status; 0 means 200 (204 without a body)
	Errors                         []int // statuses beyond the defaults
	// Pre runs before the request is parsed (path params, body); a non-zero
	// status short-circuits with that status and message.
	Pre func(ctx huma.Context) (status int, code, msg string)
}

var jsonBodyMethods = map[string]bool{http.MethodPost: true, http.MethodPut: true, http.MethodPatch: true}

// jsonContentType refuses a non-JSON Content-Type on a request that
// carries a body, like decodeJSON did; an absent one is tolerated.
func jsonContentType(ctx huma.Context) (int, string, string) {
	ct := ctx.Header("Content-Type")
	if ct == "" || ctx.Header("Content-Length") == "0" {
		return 0, "", ""
	}
	if mt, _, err := mime.ParseMediaType(ct); err != nil || mt != "application/json" {
		return http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json"
	}
	return 0, "", ""
}

// writeRaw writes a typed error response from middleware.
func writeRaw(api huma.API, ctx huma.Context, status int, code, msg string) {
	e := apiErr(status, code, msg)
	ctx.SetHeader("Content-Type", "application/json")
	ctx.SetStatus(status)
	_ = json.NewEncoder(ctx.BodyWriter()).Encode(e.ErrorResponse)
}

// register adds a typed operation to api.
func register[I, O any](api huma.API, rt route, h func(context.Context, *I) (*O, error)) {
	op := huma.Operation{
		OperationID:        rt.ID,
		Method:             rt.Method,
		Path:               rt.Path,
		Summary:            rt.Summary,
		Tags:               []string{rt.Tag},
		DefaultStatus:      rt.Status,
		SkipValidateParams: true,
		SkipValidateBody:   true,
		MaxBodyBytes:       1 << 20,
		BodyReadTimeout:    -1,
	}
	var in I
	_, hasBody := reflect.TypeOf(in).FieldByName("Body")
	if hasBody && jsonBodyMethods[rt.Method] || rt.Pre != nil {
		pre := rt.Pre
		op.Middlewares = huma.Middlewares{func(ctx huma.Context, next func(huma.Context)) {
			if hasBody && jsonBodyMethods[rt.Method] {
				if s, c, m := jsonContentType(ctx); s != 0 {
					writeRaw(api, ctx, s, c, m)
					return
				}
			}
			if pre != nil {
				if s, c, m := pre(ctx); s != 0 {
					writeRaw(api, ctx, s, c, m)
					return
				}
			}
			next(ctx)
		}}
	}
	errs := map[int]bool{http.StatusUnauthorized: true}
	if rt.Method != http.MethodGet {
		errs[http.StatusForbidden] = true
	}
	if strings.Contains(rt.Path, "{") || rt.Method != http.MethodGet {
		errs[http.StatusBadRequest] = true
	}
	if strings.Contains(rt.Path, "{") {
		errs[http.StatusNotFound] = true
	}
	for _, e := range rt.Errors {
		errs[e] = true
	}
	for c := range errs {
		op.Errors = append(op.Errors, c)
	}
	sort.Ints(op.Errors)
	huma.Register(api, op, h)
}
