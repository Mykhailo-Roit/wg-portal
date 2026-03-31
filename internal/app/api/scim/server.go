package scim

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/elimity-com/scim"
	scimerrors "github.com/elimity-com/scim/errors"
	"github.com/elimity-com/scim/optional"
	"github.com/elimity-com/scim/schema"

	"github.com/h44z/wg-portal/internal/config"
	"github.com/h44z/wg-portal/internal/domain"
)

// NewScimHandler creates a new SCIM v2.0 HTTP handler with bearer token authentication.
func NewScimHandler(cfg *config.Config, userManager UserManager) (http.Handler, error) {
	// Azure Entra ID sends booleans as strings (e.g., "False" instead of false).
	schema.SetAllowStringValues(true)

	userSchema := schema.Schema{
		ID:          schema.UserSchema,
		Name:        optional.NewString("User"),
		Description: optional.NewString("User Account"),
		Attributes: []schema.CoreAttribute{
			schema.SimpleCoreAttribute(schema.SimpleStringParams(schema.StringParams{
				Name:       "userName",
				Required:   true,
				Uniqueness: schema.AttributeUniquenessServer(),
			})),
			schema.SimpleCoreAttribute(schema.SimpleStringParams(schema.StringParams{
				Name: "displayName",
			})),
			schema.SimpleCoreAttribute(schema.SimpleBooleanParams(schema.BooleanParams{
				Name: "active",
			})),
			schema.ComplexCoreAttribute(schema.ComplexParams{
				Name: "name",
				SubAttributes: []schema.SimpleParams{
					schema.SimpleStringParams(schema.StringParams{Name: "givenName"}),
					schema.SimpleStringParams(schema.StringParams{Name: "familyName"}),
					schema.SimpleStringParams(schema.StringParams{Name: "formatted", Mutability: schema.AttributeMutabilityReadOnly()}),
				},
			}),
			schema.ComplexCoreAttribute(schema.ComplexParams{
				Name:        "emails",
				MultiValued: true,
				SubAttributes: []schema.SimpleParams{
					schema.SimpleStringParams(schema.StringParams{Name: "value"}),
					schema.SimpleStringParams(schema.StringParams{Name: "type"}),
					schema.SimpleBooleanParams(schema.BooleanParams{Name: "primary"}),
				},
			}),
			schema.ComplexCoreAttribute(schema.ComplexParams{
				Name:        "phoneNumbers",
				MultiValued: true,
				SubAttributes: []schema.SimpleParams{
					schema.SimpleStringParams(schema.StringParams{Name: "value"}),
					schema.SimpleStringParams(schema.StringParams{Name: "type"}),
				},
			}),
		},
	}

	spc := &scim.ServiceProviderConfig{
		SupportPatch:     true,
		SupportFiltering: true,
		MaxResults:       200,
		AuthenticationSchemes: []scim.AuthenticationScheme{
			{
				Type:        scim.AuthenticationTypeOauthBearerToken,
				Name:        "OAuth Bearer Token",
				Description: "Authentication scheme using the OAuth Bearer Token Standard",
				Primary:     true,
			},
		},
	}

	resourceType := scim.ResourceType{
		ID:       optional.NewString("User"),
		Name:     "User",
		Endpoint: "/Users",
		Schema:   userSchema,
		SchemaExtensions: []scim.SchemaExtension{
			{Schema: schema.ExtensionEnterpriseUser()},
		},
		Handler: &userHandler{users: userManager, cfg: cfg},
	}

	scimServer, err := scim.NewServer(&scim.ServerArgs{
		ServiceProviderConfig: spc,
		ResourceTypes:         []scim.ResourceType{resourceType},
	})
	if err != nil {
		return nil, err
	}

	return scimLoggingMiddleware(bearerTokenMiddleware(cfg.Scim.BearerToken, scimServer)), nil
}

func scimLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("[SCIM] Incoming request", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
		rw := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)
		slog.Debug("[SCIM] Response", "method", r.Method, "path", r.URL.Path, "status", rw.statusCode)
	})
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseRecorder) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func bearerTokenMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			slog.Debug("[SCIM] Auth failed: bearer token not configured", "method", r.Method, "path", r.URL.Path)
			writeScimUnauthorized(w)
			return
		}

		authHeader := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(authHeader, prefix) {
			slog.Debug("[SCIM] Auth failed: missing Bearer prefix", "method", r.Method, "path", r.URL.Path)
			writeScimUnauthorized(w)
			return
		}

		provided := authHeader[len(prefix):]
		if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			slog.Debug("[SCIM] Auth failed: invalid token", "method", r.Method, "path", r.URL.Path)
			writeScimUnauthorized(w)
			return
		}

		ctx := domain.SetUserInfo(r.Context(), domain.SystemAdminContextUserInfo())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeScimUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/scim+json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(scimerrors.ScimError{
		Detail: "Authorization required",
		Status: http.StatusUnauthorized,
	})
}
