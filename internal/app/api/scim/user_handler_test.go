package scim

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/h44z/wg-portal/internal/config"
	"github.com/h44z/wg-portal/internal/domain"
)

// --- Mock ---

type mockUserManager struct {
	users map[domain.UserIdentifier]*domain.User
}

func newMockUserManager() *mockUserManager {
	return &mockUserManager{users: make(map[domain.UserIdentifier]*domain.User)}
}

func (m *mockUserManager) GetUser(_ context.Context, id domain.UserIdentifier) (*domain.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, fmt.Errorf("user %s: %w", id, domain.ErrNotFound)
	}
	cp := *u
	return &cp, nil
}

func (m *mockUserManager) GetAllUsers(_ context.Context) ([]domain.User, error) {
	out := make([]domain.User, 0, len(m.users))
	for _, u := range m.users {
		out = append(out, *u)
	}
	return out, nil
}

func (m *mockUserManager) CreateUser(_ context.Context, user *domain.User) (*domain.User, error) {
	if _, exists := m.users[user.Identifier]; exists {
		return nil, fmt.Errorf("user %s: %w", user.Identifier, domain.ErrDuplicateEntry)
	}
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now
	m.users[user.Identifier] = user
	cp := *user
	return &cp, nil
}

func (m *mockUserManager) UpdateUser(_ context.Context, user *domain.User) (*domain.User, error) {
	if _, exists := m.users[user.Identifier]; !exists {
		return nil, fmt.Errorf("user %s: %w", user.Identifier, domain.ErrNotFound)
	}
	user.UpdatedAt = time.Now()
	m.users[user.Identifier] = user
	cp := *user
	return &cp, nil
}

func (m *mockUserManager) DeleteUser(_ context.Context, id domain.UserIdentifier) error {
	if _, exists := m.users[id]; !exists {
		return fmt.Errorf("user %s: %w", id, domain.ErrNotFound)
	}
	delete(m.users, id)
	return nil
}

// --- Helpers ---

const testToken = "test-scim-token"

func testCfg(deleteAction string) *config.Config {
	cfg := &config.Config{}
	cfg.Scim.Enabled = true
	cfg.Scim.BearerToken = testToken
	cfg.Scim.DeleteAction = deleteAction
	return cfg
}

func newTestHandler(t *testing.T, deleteAction string) (http.Handler, *mockUserManager) {
	t.Helper()
	mock := newMockUserManager()
	cfg := testCfg(deleteAction)
	h, err := NewScimHandler(cfg, mock)
	require.NoError(t, err)
	return h, mock
}

func doRequest(handler http.Handler, method, target string, body string) *httptest.ResponseRecorder {
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	var req *http.Request
	if reader != nil {
		req = httptest.NewRequest(method, target, reader)
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	if body != "" {
		req.Header.Set("Content-Type", "application/scim+json")
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func parseBody(t *testing.T, rr *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	d := json.NewDecoder(rr.Body)
	d.UseNumber()
	require.NoError(t, d.Decode(&m))
	return m
}

// --- Bearer Token Middleware Tests ---

func TestBearerToken_NoHeader(t *testing.T) {
	h, _ := newTestHandler(t, "disable")
	req := httptest.NewRequest(http.MethodGet, "/v2/ServiceProviderConfig", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestBearerToken_WrongToken(t *testing.T) {
	h, _ := newTestHandler(t, "disable")
	req := httptest.NewRequest(http.MethodGet, "/v2/ServiceProviderConfig", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestBearerToken_BasicScheme(t *testing.T) {
	h, _ := newTestHandler(t, "disable")
	req := httptest.NewRequest(http.MethodGet, "/v2/ServiceProviderConfig", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestBearerToken_Valid(t *testing.T) {
	h, _ := newTestHandler(t, "disable")
	rr := doRequest(h, http.MethodGet, "/v2/ServiceProviderConfig", "")
	assert.Equal(t, http.StatusOK, rr.Code)
}

// --- CRUD Tests ---

const validUserJSON = `{
	"schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
	"userName": "jdoe",
	"name": {"givenName": "John", "familyName": "Doe"},
	"emails": [{"value": "jdoe@example.com", "primary": true}],
	"active": true
}`

func TestCreateUser_Valid(t *testing.T) {
	h, mock := newTestHandler(t, "disable")
	rr := doRequest(h, http.MethodPost, "/v2/Users", validUserJSON)
	assert.Equal(t, http.StatusCreated, rr.Code)

	body := parseBody(t, rr)
	assert.Equal(t, "jdoe", body["id"])
	assert.Equal(t, "jdoe", body["userName"])
	assert.NotNil(t, body["meta"])

	// Verify stored
	assert.Contains(t, mock.users, domain.UserIdentifier("jdoe"))
	assert.Equal(t, "John", mock.users[domain.UserIdentifier("jdoe")].Firstname)
}

func TestCreateUser_MissingUserName(t *testing.T) {
	h, _ := newTestHandler(t, "disable")
	rr := doRequest(h, http.MethodPost, "/v2/Users", `{
		"schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
		"name": {"givenName": "John"}
	}`)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestCreateUser_Duplicate(t *testing.T) {
	h, _ := newTestHandler(t, "disable")
	doRequest(h, http.MethodPost, "/v2/Users", validUserJSON)
	// Second create for same user updates profile instead of returning conflict
	rr := doRequest(h, http.MethodPost, "/v2/Users", validUserJSON)
	assert.Equal(t, http.StatusCreated, rr.Code)
}

func TestGetUser_Exists(t *testing.T) {
	h, _ := newTestHandler(t, "disable")
	doRequest(h, http.MethodPost, "/v2/Users", validUserJSON)

	rr := doRequest(h, http.MethodGet, "/v2/Users/jdoe", "")
	assert.Equal(t, http.StatusOK, rr.Code)
	body := parseBody(t, rr)
	assert.Equal(t, "jdoe", body["userName"])
}

func TestGetUser_NotFound(t *testing.T) {
	h, _ := newTestHandler(t, "disable")
	rr := doRequest(h, http.MethodGet, "/v2/Users/missing", "")
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestGetAllUsers(t *testing.T) {
	h, _ := newTestHandler(t, "disable")
	doRequest(h, http.MethodPost, "/v2/Users", validUserJSON)

	rr := doRequest(h, http.MethodGet, "/v2/Users", "")
	assert.Equal(t, http.StatusOK, rr.Code)
	body := parseBody(t, rr)
	assert.Equal(t, json.Number("1"), body["totalResults"])
}

func TestGetAllUsers_Filter(t *testing.T) {
	h, _ := newTestHandler(t, "disable")
	doRequest(h, http.MethodPost, "/v2/Users", validUserJSON)
	doRequest(h, http.MethodPost, "/v2/Users", `{
		"schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
		"userName": "other"
	}`)

	req := httptest.NewRequest(http.MethodGet, "/v2/Users", nil)
	q := req.URL.Query()
	q.Set("filter", `userName eq "jdoe"`)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Authorization", "Bearer "+testToken)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	body := parseBody(t, rr)
	assert.Equal(t, json.Number("1"), body["totalResults"])
}

func TestReplaceUser(t *testing.T) {
	h, mock := newTestHandler(t, "disable")
	doRequest(h, http.MethodPost, "/v2/Users", validUserJSON)

	rr := doRequest(h, http.MethodPut, "/v2/Users/jdoe", `{
		"schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
		"userName": "jdoe",
		"name": {"givenName": "Jane", "familyName": "Doe"},
		"active": true
	}`)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "Jane", mock.users[domain.UserIdentifier("jdoe")].Firstname)
}

func TestPatchUser_Deactivate(t *testing.T) {
	h, mock := newTestHandler(t, "disable")
	doRequest(h, http.MethodPost, "/v2/Users", validUserJSON)

	rr := doRequest(h, http.MethodPatch, "/v2/Users/jdoe", `{
		"schemas": ["urn:ietf:params:scim:api:messages:2.0:PatchOp"],
		"Operations": [{"op": "replace", "value": {"active": false}}]
	}`)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.NotNil(t, mock.users[domain.UserIdentifier("jdoe")].Disabled)
}

func TestDeleteUser_Disable(t *testing.T) {
	h, mock := newTestHandler(t, "disable")
	doRequest(h, http.MethodPost, "/v2/Users", validUserJSON)

	rr := doRequest(h, http.MethodDelete, "/v2/Users/jdoe", "")
	assert.Equal(t, http.StatusNoContent, rr.Code)
	// User still exists but is disabled
	assert.Contains(t, mock.users, domain.UserIdentifier("jdoe"))
	assert.NotNil(t, mock.users[domain.UserIdentifier("jdoe")].Disabled)
	assert.Equal(t, "SCIM deprovisioned", mock.users[domain.UserIdentifier("jdoe")].DisabledReason)
}

func TestDeleteUser_HardDelete(t *testing.T) {
	h, mock := newTestHandler(t, "delete")
	doRequest(h, http.MethodPost, "/v2/Users", validUserJSON)

	rr := doRequest(h, http.MethodDelete, "/v2/Users/jdoe", "")
	assert.Equal(t, http.StatusNoContent, rr.Code)
	assert.NotContains(t, mock.users, domain.UserIdentifier("jdoe"))
}

func TestDeleteUser_NotFound(t *testing.T) {
	h, _ := newTestHandler(t, "delete")
	rr := doRequest(h, http.MethodDelete, "/v2/Users/missing", "")
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// --- Helper Function Tests ---

func TestDomainUserToResource_RoundTrip(t *testing.T) {
	user := &domain.User{
		Identifier: "test",
		Firstname:  "John",
		Lastname:   "Doe",
		Email:      "john@example.com",
		Phone:      "+1234567890",
		ExternalId: "ext-123",
	}

	res := domainUserToResource(user)
	assert.Equal(t, "test", res.ID)
	assert.Equal(t, true, res.Attributes["active"])
	assert.Equal(t, "ext-123", res.ExternalID.Value())

	// Convert back
	back := attributesToDomainUser(res.Attributes)
	assert.Equal(t, user.Identifier, back.Identifier)
	assert.Equal(t, user.Firstname, back.Firstname)
	assert.Equal(t, user.Lastname, back.Lastname)
	assert.Equal(t, user.Email, back.Email)
	assert.Nil(t, back.Disabled)
}

func TestAttributesToDomainUser_ActiveFalse(t *testing.T) {
	attrs := map[string]interface{}{
		"userName": "test",
		"active":   false,
	}
	user := attributesToDomainUser(attrs)
	assert.NotNil(t, user.Disabled)
}

func TestExtractPrimaryEmail(t *testing.T) {
	emails := []interface{}{
		map[string]interface{}{"value": "secondary@example.com", "primary": false},
		map[string]interface{}{"value": "primary@example.com", "primary": true},
	}
	assert.Equal(t, "primary@example.com", extractPrimaryEmail(emails))

	// Fallback to first when no primary
	emails2 := []interface{}{
		map[string]interface{}{"value": "first@example.com"},
		map[string]interface{}{"value": "second@example.com"},
	}
	assert.Equal(t, "first@example.com", extractPrimaryEmail(emails2))
}

// --- New Tests for SCIM Enhancements ---

func TestBearerToken_EmptyConfigured(t *testing.T) {
	cfg := &config.Config{}
	cfg.Scim.Enabled = true
	cfg.Scim.BearerToken = "" // empty token
	mock := newMockUserManager()
	h, err := NewScimHandler(cfg, mock)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/v2/Users", nil)
	req.Header.Set("Authorization", "Bearer anything")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestCreateUser_ExistingUserUpdatesProfile(t *testing.T) {
	h, mock := newTestHandler(t, "disable")
	// Pre-create user (simulating OIDC login)
	mock.users[domain.UserIdentifier("jdoe")] = &domain.User{
		Identifier: "jdoe",
		Firstname:  "Old",
		Lastname:   "Name",
		Email:      "old@example.com",
	}

	rr := doRequest(h, http.MethodPost, "/v2/Users", validUserJSON)
	assert.Equal(t, http.StatusCreated, rr.Code)

	u := mock.users[domain.UserIdentifier("jdoe")]
	assert.Equal(t, "John", u.Firstname)
	assert.Equal(t, "Doe", u.Lastname)
	assert.Equal(t, "jdoe@example.com", u.Email)
}

func TestCreateUser_ExistingUserSyncsDepartment(t *testing.T) {
	h, mock := newTestHandler(t, "disable")
	mock.users[domain.UserIdentifier("jdoe")] = &domain.User{
		Identifier: "jdoe",
	}

	rr := doRequest(h, http.MethodPost, "/v2/Users", `{
		"schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
		"userName": "jdoe",
		"active": true,
		"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User": {"department": "Engineering"}
	}`)
	assert.Equal(t, http.StatusCreated, rr.Code)
	assert.Equal(t, "Engineering", mock.users[domain.UserIdentifier("jdoe")].Department)
}

func TestCreateUser_ExistingUserSyncsDisabledReason(t *testing.T) {
	h, mock := newTestHandler(t, "disable")
	mock.users[domain.UserIdentifier("jdoe")] = &domain.User{
		Identifier: "jdoe",
	}

	rr := doRequest(h, http.MethodPost, "/v2/Users", `{
		"schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
		"userName": "jdoe",
		"active": false
	}`)
	assert.Equal(t, http.StatusCreated, rr.Code)
	u := mock.users[domain.UserIdentifier("jdoe")]
	assert.NotNil(t, u.Disabled)
	assert.Equal(t, "SCIM provisioned disabled", u.DisabledReason)
}

func TestCreateUser_ProviderName(t *testing.T) {
	mock := newMockUserManager()
	cfg := testCfg("disable")
	cfg.Scim.ProviderName = "MyOIDC"
	h, err := NewScimHandler(cfg, mock)
	require.NoError(t, err)

	doRequest(h, http.MethodPost, "/v2/Users", validUserJSON)
	u := mock.users[domain.UserIdentifier("jdoe")]
	require.Len(t, u.Authentications, 1)
	assert.Equal(t, "MyOIDC", u.Authentications[0].ProviderName)
	assert.Equal(t, domain.UserSourceOauth, u.Authentications[0].Source)
}

func TestCreateUser_DefaultProviderName(t *testing.T) {
	h, mock := newTestHandler(t, "disable")
	doRequest(h, http.MethodPost, "/v2/Users", validUserJSON)
	u := mock.users[domain.UserIdentifier("jdoe")]
	require.Len(t, u.Authentications, 1)
	assert.Equal(t, "scim", u.Authentications[0].ProviderName)
}

func TestPatchUser_DisabledReason(t *testing.T) {
	h, mock := newTestHandler(t, "disable")
	doRequest(h, http.MethodPost, "/v2/Users", validUserJSON)

	doRequest(h, http.MethodPatch, "/v2/Users/jdoe", `{
		"schemas": ["urn:ietf:params:scim:api:messages:2.0:PatchOp"],
		"Operations": [{"op": "replace", "value": {"active": false}}]
	}`)
	u := mock.users[domain.UserIdentifier("jdoe")]
	assert.NotNil(t, u.Disabled)
	assert.Equal(t, "SCIM provisioned disabled", u.DisabledReason)

	// Re-enable clears reason
	doRequest(h, http.MethodPatch, "/v2/Users/jdoe", `{
		"schemas": ["urn:ietf:params:scim:api:messages:2.0:PatchOp"],
		"Operations": [{"op": "replace", "value": {"active": true}}]
	}`)
	u = mock.users[domain.UserIdentifier("jdoe")]
	assert.Nil(t, u.Disabled)
	assert.Equal(t, "", u.DisabledReason)
}

func TestPatchUser_FilteredPhonePath(t *testing.T) {
	h, mock := newTestHandler(t, "disable")
	doRequest(h, http.MethodPost, "/v2/Users", validUserJSON)

	rr := doRequest(h, http.MethodPatch, "/v2/Users/jdoe", `{
		"schemas": ["urn:ietf:params:scim:api:messages:2.0:PatchOp"],
		"Operations": [{"op": "Add", "path": "phoneNumbers[type eq \"mobile\"].value", "value": "+1555123"}]
	}`)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "+1555123", mock.users[domain.UserIdentifier("jdoe")].Phone)
}

func TestPatchUser_FilteredEmailPath(t *testing.T) {
	h, mock := newTestHandler(t, "disable")
	doRequest(h, http.MethodPost, "/v2/Users", validUserJSON)

	rr := doRequest(h, http.MethodPatch, "/v2/Users/jdoe", `{
		"schemas": ["urn:ietf:params:scim:api:messages:2.0:PatchOp"],
		"Operations": [{"op": "Add", "path": "emails[type eq \"work\"].value", "value": "work@example.com"}]
	}`)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "work@example.com", mock.users[domain.UserIdentifier("jdoe")].Email)
}

func TestPatchUser_EnterpriseDepartment(t *testing.T) {
	h, mock := newTestHandler(t, "disable")
	doRequest(h, http.MethodPost, "/v2/Users", validUserJSON)

	rr := doRequest(h, http.MethodPatch, "/v2/Users/jdoe", `{
		"schemas": ["urn:ietf:params:scim:api:messages:2.0:PatchOp"],
		"Operations": [{"op": "replace", "path": "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department", "value": "Sales"}]
	}`)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "Sales", mock.users[domain.UserIdentifier("jdoe")].Department)
}

func TestGetAllUsers_EmptyResourcesNotNull(t *testing.T) {
	h, _ := newTestHandler(t, "disable")
	rr := doRequest(h, http.MethodGet, "/v2/Users", "")
	assert.Equal(t, http.StatusOK, rr.Code)
	// Verify Resources is [] not null
	assert.Contains(t, rr.Body.String(), `"Resources":[]`)
}

func TestDomainUserToResource_Department(t *testing.T) {
	user := &domain.User{
		Identifier: "test",
		Department: "Engineering",
	}
	res := domainUserToResource(user)
	enterprise, ok := res.Attributes[enterpriseSchemaID].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "Engineering", enterprise["department"])
}

func TestDomainUserToResource_NoDepartment(t *testing.T) {
	user := &domain.User{
		Identifier: "test",
	}
	res := domainUserToResource(user)
	_, ok := res.Attributes[enterpriseSchemaID]
	assert.False(t, ok, "enterprise extension should not be present when department is empty")
}

func TestAttributesToDomainUser_Department(t *testing.T) {
	attrs := map[string]interface{}{
		"userName":          "test",
		enterpriseSchemaID: map[string]interface{}{"department": "Sales"},
	}
	user := attributesToDomainUser(attrs)
	assert.Equal(t, "Sales", user.Department)
}

func TestClearField_Department(t *testing.T) {
	user := &domain.User{Department: "Engineering"}
	clearField(user, enterpriseSchemaID+":department")
	assert.Equal(t, "", user.Department)
}
