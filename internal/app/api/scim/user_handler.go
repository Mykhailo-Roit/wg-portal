package scim

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/elimity-com/scim"
	scimerrors "github.com/elimity-com/scim/errors"
	"github.com/elimity-com/scim/optional"

	"github.com/h44z/wg-portal/internal/config"
	"github.com/h44z/wg-portal/internal/domain"
)

// UserManager defines the subset of users.Manager methods needed by the SCIM handler.
type UserManager interface {
	GetUser(ctx context.Context, id domain.UserIdentifier) (*domain.User, error)
	GetAllUsers(ctx context.Context) ([]domain.User, error)
	CreateUser(ctx context.Context, user *domain.User) (*domain.User, error)
	UpdateUser(ctx context.Context, user *domain.User) (*domain.User, error)
	DeleteUser(ctx context.Context, id domain.UserIdentifier) error
}

type userHandler struct {
	users UserManager
	cfg   *config.Config
}

func (h *userHandler) Create(r *http.Request, attrs scim.ResourceAttributes) (scim.Resource, error) {
	slog.Debug("[SCIM] Create user request", "userName", attrs["userName"], "externalId", attrs["externalId"])
	user := attributesToDomainUser(attrs)

	// SCIM-provisioned users authenticate via OIDC, not local database passwords.
	now := time.Now()
	ctxUserInfo := domain.GetUserInfo(r.Context())
	user.Authentications = []domain.UserAuthentication{
		{
			BaseModel: domain.BaseModel{
				CreatedBy: ctxUserInfo.UserId(),
				UpdatedBy: ctxUserInfo.UserId(),
				CreatedAt: now,
				UpdatedAt: now,
			},
			UserIdentifier: user.Identifier,
			Source:         domain.UserSourceOauth,
			ProviderName:   "scim",
		},
	}

	created, err := h.users.CreateUser(r.Context(), user)
	if err != nil {
		slog.Debug("[SCIM] Create user failed", "userName", user.Identifier, "error", err)
		if errors.Is(err, domain.ErrDuplicateEntry) {
			return scim.Resource{}, scimerrors.ScimErrorUniqueness
		}
		return scim.Resource{}, scimerrors.ScimErrorInternal
	}
	slog.Debug("[SCIM] Create user success", "id", created.Identifier, "email", created.Email)
	return domainUserToResource(created), nil
}

func (h *userHandler) Get(r *http.Request, id string) (scim.Resource, error) {
	slog.Debug("[SCIM] Get user request", "id", id)
	user, err := h.users.GetUser(r.Context(), domain.UserIdentifier(id))
	if err != nil {
		slog.Debug("[SCIM] Get user failed", "id", id, "error", err)
		if errors.Is(err, domain.ErrNotFound) {
			return scim.Resource{}, scimerrors.ScimErrorResourceNotFound(id)
		}
		return scim.Resource{}, scimerrors.ScimErrorInternal
	}
	return domainUserToResource(user), nil
}

func (h *userHandler) GetAll(r *http.Request, params scim.ListRequestParams) (scim.Page, error) {
	slog.Debug("[SCIM] GetAll users request", "startIndex", params.StartIndex, "count", params.Count)
	users, err := h.users.GetAllUsers(r.Context())
	if err != nil {
		slog.Debug("[SCIM] GetAll users failed", "error", err)
		return scim.Page{}, scimerrors.ScimErrorInternal
	}

	// Filter
	filtered := make([]scim.Resource, 0)
	for i := range users {
		res := domainUserToResource(&users[i])
		if params.FilterValidator != nil {
			if err := params.FilterValidator.PassesFilter(res.Attributes); err != nil {
				continue
			}
		}
		filtered = append(filtered, res)
	}

	total := len(filtered)

	// Paginate (startIndex is 1-based)
	start := params.StartIndex - 1
	if start < 0 {
		start = 0
	}
	if start > total {
		start = total
	}
	end := start + params.Count
	if end > total {
		end = total
	}

	return scim.Page{
		TotalResults: total,
		Resources:    filtered[start:end],
	}, nil
}

func (h *userHandler) Replace(r *http.Request, id string, attrs scim.ResourceAttributes) (scim.Resource, error) {
	slog.Debug("[SCIM] Replace user request", "id", id, "userName", attrs["userName"])
	user := attributesToDomainUser(attrs)
	user.Identifier = domain.UserIdentifier(id)
	updated, err := h.users.UpdateUser(r.Context(), user)
	if err != nil {
		slog.Debug("[SCIM] Replace user failed", "id", id, "error", err)
		if errors.Is(err, domain.ErrNotFound) {
			return scim.Resource{}, scimerrors.ScimErrorResourceNotFound(id)
		}
		return scim.Resource{}, scimerrors.ScimErrorInternal
	}
	slog.Debug("[SCIM] Replace user success", "id", id)
	return domainUserToResource(updated), nil
}

func (h *userHandler) Patch(r *http.Request, id string, operations []scim.PatchOperation) (scim.Resource, error) {
	slog.Debug("[SCIM] Patch user request", "id", id, "operations", len(operations))
	existing, err := h.users.GetUser(r.Context(), domain.UserIdentifier(id))
	if err != nil {
		slog.Debug("[SCIM] Patch user lookup failed", "id", id, "error", err)
		if errors.Is(err, domain.ErrNotFound) {
			return scim.Resource{}, scimerrors.ScimErrorResourceNotFound(id)
		}
		return scim.Resource{}, scimerrors.ScimErrorInternal
	}

	for _, op := range operations {
		slog.Debug("[SCIM] Patch operation", "id", id, "op", op.Op, "path", op.Path)
		switch op.Op {
		case scim.PatchOperationAdd, scim.PatchOperationReplace:
			if op.Path != nil {
				applyPatchPath(existing, op.Path.String(), op.Value)
			} else if m, ok := op.Value.(map[string]interface{}); ok {
				for k, v := range m {
					applyPatchPath(existing, k, v)
				}
			}
		case scim.PatchOperationRemove:
			if op.Path != nil {
				clearField(existing, op.Path.String())
			}
		}
	}

	updated, err := h.users.UpdateUser(r.Context(), existing)
	if err != nil {
		slog.Debug("[SCIM] Patch user update failed", "id", id, "error", err)
		return scim.Resource{}, scimerrors.ScimErrorInternal
	}
	slog.Debug("[SCIM] Patch user success", "id", id)
	return domainUserToResource(updated), nil
}

func (h *userHandler) Delete(r *http.Request, id string) error {
	slog.Debug("[SCIM] Delete user request", "id", id, "action", h.cfg.Scim.DeleteAction)
	if h.cfg.Scim.DeleteAction == "delete" {
		err := h.users.DeleteUser(r.Context(), domain.UserIdentifier(id))
		if err != nil {
			slog.Debug("[SCIM] Delete user failed", "id", id, "error", err)
			if errors.Is(err, domain.ErrNotFound) {
				return scimerrors.ScimErrorResourceNotFound(id)
			}
			return scimerrors.ScimErrorInternal
		}
		slog.Debug("[SCIM] Delete user success (hard delete)", "id", id)
		return nil
	}

	// Default: disable
	user, err := h.users.GetUser(r.Context(), domain.UserIdentifier(id))
	if err != nil {
		slog.Debug("[SCIM] Delete user lookup failed", "id", id, "error", err)
		if errors.Is(err, domain.ErrNotFound) {
			return scimerrors.ScimErrorResourceNotFound(id)
		}
		return scimerrors.ScimErrorInternal
	}
	now := time.Now()
	user.Disabled = &now
	user.DisabledReason = "SCIM deprovisioned"
	if _, err := h.users.UpdateUser(r.Context(), user); err != nil {
		slog.Debug("[SCIM] Delete user disable failed", "id", id, "error", err)
		return scimerrors.ScimErrorInternal
	}
	slog.Debug("[SCIM] Delete user success (disabled)", "id", id)
	return nil
}

// domainUserToResource converts a domain.User to a scim.Resource.
func domainUserToResource(user *domain.User) scim.Resource {
	attrs := scim.ResourceAttributes{
		"userName":    string(user.Identifier),
		"active":      user.Disabled == nil,
		"displayName": user.DisplayName(),
		"name": map[string]interface{}{
			"givenName":  user.Firstname,
			"familyName": user.Lastname,
		},
	}

	if user.Email != "" {
		attrs["emails"] = []interface{}{
			map[string]interface{}{"value": user.Email, "primary": true},
		}
	}
	if user.Phone != "" {
		attrs["phoneNumbers"] = []interface{}{
			map[string]interface{}{"value": user.Phone},
		}
	}

	res := scim.Resource{
		ID:         string(user.Identifier),
		Attributes: attrs,
		Meta: scim.Meta{
			Created:      &user.CreatedAt,
			LastModified: &user.UpdatedAt,
		},
	}
	if user.ExternalId != "" {
		res.ExternalID = optional.NewString(user.ExternalId)
	}
	return res
}

// attributesToDomainUser converts SCIM ResourceAttributes to a domain.User.
func attributesToDomainUser(attrs scim.ResourceAttributes) *domain.User {
	user := &domain.User{}

	if v, ok := attrs["userName"].(string); ok {
		user.Identifier = domain.UserIdentifier(v)
	}
	if v, ok := attrs["externalId"].(string); ok {
		user.ExternalId = v
	}
	if name, ok := attrs["name"].(map[string]interface{}); ok {
		if v, ok := name["givenName"].(string); ok {
			user.Firstname = v
		}
		if v, ok := name["familyName"].(string); ok {
			user.Lastname = v
		}
	}
	if emails, ok := attrs["emails"].([]interface{}); ok {
		user.Email = extractPrimaryEmail(emails)
	}
	if phones, ok := attrs["phoneNumbers"].([]interface{}); ok {
		if len(phones) > 0 {
			if m, ok := phones[0].(map[string]interface{}); ok {
				if v, ok := m["value"].(string); ok {
					user.Phone = v
				}
			}
		}
	}
	if active, ok := attrs["active"].(bool); ok && !active {
		now := time.Now()
		user.Disabled = &now
	}

	return user
}

func extractPrimaryEmail(emails []interface{}) string {
	var first string
	for _, e := range emails {
		m, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		val, _ := m["value"].(string)
		if first == "" {
			first = val
		}
		if primary, ok := m["primary"].(bool); ok && primary {
			return val
		}
	}
	return first
}

func applyPatchPath(user *domain.User, path string, value interface{}) {
	switch path {
	case "active":
		if active, ok := value.(bool); ok {
			if active {
				user.Disabled = nil
				user.DisabledReason = ""
			} else {
				now := time.Now()
				user.Disabled = &now
			}
		}
	case "userName":
		if v, ok := value.(string); ok {
			user.Identifier = domain.UserIdentifier(v)
		}
	case "name.givenName":
		if v, ok := value.(string); ok {
			user.Firstname = v
		}
	case "name.familyName":
		if v, ok := value.(string); ok {
			user.Lastname = v
		}
	case "externalId":
		if v, ok := value.(string); ok {
			user.ExternalId = v
		}
	case "emails":
		if emails, ok := value.([]interface{}); ok {
			user.Email = extractPrimaryEmail(emails)
		}
	case "phoneNumbers":
		if phones, ok := value.([]interface{}); ok && len(phones) > 0 {
			if m, ok := phones[0].(map[string]interface{}); ok {
				if v, ok := m["value"].(string); ok {
					user.Phone = v
				}
			}
		}
	}
}

func clearField(user *domain.User, path string) {
	switch path {
	case "name.givenName":
		user.Firstname = ""
	case "name.familyName":
		user.Lastname = ""
	case "emails":
		user.Email = ""
	case "phoneNumbers":
		user.Phone = ""
	case "externalId":
		user.ExternalId = ""
	}
}
