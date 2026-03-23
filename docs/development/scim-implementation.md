# SCIM v2.0 Implementation Plan

SCIM (System for Cross-domain Identity Management) provisioning for WG-Portal using bearer token authentication.

- RFC: https://datatracker.ietf.org/doc/html/rfc7644
- Library: https://github.com/elimity-com/scim

## Library Overview

`elimity-com/scim` provides a `Server` that implements `http.Handler`. It handles:
- Routing: `/Users`, `/Users/{id}`, `/Schemas`, `/ResourceTypes`, `/ServiceProviderConfig`
- `/v2` prefix stripping (built-in)
- Schema validation on POST/PUT/PATCH bodies
- Filter parsing and validation via `filter.Validator`
- PATCH operation parsing (add/remove/replace with path validation)
- Pagination (`startIndex`, `count`)
- `application/scim+json` content type
- SCIM error responses (`urn:ietf:params:scim:api:messages:2.0:Error`)

You implement one interface per resource type:

```go
// github.com/elimity-com/scim
type ResourceHandler interface {
    Create(r *http.Request, attributes ResourceAttributes) (Resource, error)
    Get(r *http.Request, id string) (Resource, error)
    GetAll(r *http.Request, params ListRequestParams) (Page, error)
    Replace(r *http.Request, id string, attributes ResourceAttributes) (Resource, error)
    Delete(r *http.Request, id string) error
    Patch(r *http.Request, id string, operations []PatchOperation) (Resource, error)
}
```

Key library types:
- `ResourceAttributes` = `map[string]interface{}` — the validated attribute map
- `Resource` — has `ID string`, `ExternalID optional.String`, `Attributes ResourceAttributes`, `Meta Meta`
- `Page` — has `TotalResults int`, `Resources []Resource`
- `ListRequestParams` — has `Count int`, `StartIndex int`, `FilterValidator *filter.Validator`
- `PatchOperation` — has `Op string` ("add"/"remove"/"replace"), `Path *filter.Path`, `Value interface{}`
- `filter.Validator` — has `PassesFilter(map[string]interface{}) error` to test a resource against the filter

Mounting with a custom base path:
```go
http.Handle("/scim/", http.StripPrefix("/scim", scimServer))
```

## File Structure

```
internal/config/config.go                  — add Scim config section
internal/domain/user.go                    — add ExternalId field
internal/app/api/scim/server.go            — SCIM server setup + bearer token middleware
internal/app/api/scim/user_handler.go      — ResourceHandler implementation
internal/app/api/core/server.go            — mount SCIM handler
cmd/wg-portal/main.go                      — wire SCIM handler
```

## Attribute Mapping

| SCIM attribute           | ResourceAttributes key path     | domain.User field          | Notes                          |
|--------------------------|---------------------------------|----------------------------|--------------------------------|
| `id`                     | (auto by library)               | `Identifier`               | string, primary key            |
| `externalId`             | `"externalId"`                  | `ExternalId`               | new field, indexed             |
| `userName`               | `"userName"`                    | `Identifier`               | required, unique               |
| `name.givenName`         | `"name"."givenName"`            | `Firstname`                |                                |
| `name.familyName`        | `"name"."familyName"`           | `Lastname`                 |                                |
| `emails[0].value`        | `"emails"[0]."value"`           | `Email`                    | take primary or first          |
| `phoneNumbers[0].value`  | `"phoneNumbers"[0]."value"`     | `Phone`                    | take primary or first          |
| `active`                 | `"active"`                      | `Disabled` (inverted)      | `false` → set Disabled to now  |
| `displayName`            | `"displayName"`                 | computed Firstname+Lastname| read-only in SCIM              |

## Relevant Codebase References

### domain.User (`internal/domain/user.go:37-75`)
```go
type User struct {
    BaseModel
    Identifier   UserIdentifier  // primary key, string
    Email        string
    IsAdmin      bool
    Firstname    string
    Lastname     string
    Phone        string
    Department   string
    Notes        string
    Password     PrivateString
    Disabled     *time.Time      // nil = active, set = disabled
    // ... other fields
}
```

### users.Manager (`internal/app/users/user_manager.go`)
All methods require admin context (`domain.ValidateAdminAccessRights`).
- `GetUser(ctx, id UserIdentifier) (*User, error)`
- `GetAllUsers(ctx) ([]User, error)` — loads all users, enriches with peer count
- `CreateUser(ctx, *User) (*User, error)`
- `UpdateUser(ctx, *User) (*User, error)` — calls `CopyCalculatedAttributes` internally
- `DeleteUser(ctx, id UserIdentifier) error`

### Context (`internal/domain/context.go`)
```go
domain.SetUserInfo(ctx, domain.SystemAdminContextUserInfo()) // injects admin context
```

### Errors (`internal/domain/errors.go`)
```go
var ErrNotFound = errors.New("record not found")
```
Map to `errors.ScimErrorResourceNotFound(id)` from the SCIM library.

### Server (`internal/app/api/core/server.go`)
- `Server.root` is a `*routegroup.Bundle` (wraps `http.ServeMux`)
- Endpoints are mounted via `s.root.Mount(path)` or `s.root.Handle(pattern, handler)`
- The SCIM handler should be mounted on `s.root` since it's an `http.Handler`

### main.go (`cmd/wg-portal/main.go`)
- `userManager` is `*users.Manager` — pass to SCIM handler
- `cfg` is `*config.Config` — read SCIM config from here
- Server created at: `core.NewServer(cfg, apiFrontend, apiV1)`

---

## Tasks

### Task 1: Config + Domain Changes

**Files:** `internal/config/config.go`, `internal/domain/user.go`

1. Add SCIM config section to `Config` struct in `internal/config/config.go`:
```go
Scim struct {
    Enabled          bool   `yaml:"enabled"`
    BearerToken      string `yaml:"bearer_token"`
    DeleteAction     string `yaml:"delete_action"` // "disable" (default) or "delete"
} `yaml:"scim"`
```

2. Add `ExternalId` field to `domain.User` in `internal/domain/user.go`:
```go
ExternalId string `gorm:"column:external_id;index" form:"external_id"`
```
GORM auto-migrate will handle the DB migration.

3. Set defaults in `defaultConfig()` in `internal/config/config.go`:
```go
cfg.Scim.Enabled = false
cfg.Scim.DeleteAction = "disable"
```

---

### Task 2: SCIM Server Setup + Bearer Token Middleware

**File:** `internal/app/api/scim/server.go`

**Dependencies:** Task 1

1. Add `go get github.com/elimity-com/scim`

2. Create `NewScimHandler(cfg *config.Config, userManager) (http.Handler, error)` that:
   - Builds the SCIM `schema.Schema` for User (see schema definition below)
   - Creates `scim.ServiceProviderConfig` with `SupportPatch: true`, `SupportFiltering: true`, `MaxResults: 200`
   - Sets `AuthenticationSchemes` with type `scim.AuthenticationTypeOauthBearerToken`
   - Creates `scim.ResourceType` for User at endpoint `/Users`
   - Calls `scim.NewServer(&scim.ServerArgs{...})`
   - Wraps result with bearer token middleware

3. Bearer token middleware:
   - Extract `Authorization: Bearer <token>` header
   - Constant-time compare against `cfg.Scim.BearerToken` (`crypto/subtle.ConstantTimeCompare`)
   - On failure: return 401 with SCIM error JSON (`urn:ietf:params:scim:api:messages:2.0:Error`)
   - On success: inject admin context via `domain.SetUserInfo(r.Context(), domain.SystemAdminContextUserInfo())`
   - Pass to next handler

4. User schema definition — declare these SCIM attributes:
   - `userName` (string, required, uniqueness: server)
   - `displayName` (string)
   - `active` (boolean)
   - `name` (complex: `givenName`, `familyName`, `formatted`)
   - `emails` (complex, multi-valued: `value`, `type`, `primary`)
   - `phoneNumbers` (complex, multi-valued: `value`, `type`)

---

### Task 3: User ResourceHandler Implementation

**File:** `internal/app/api/scim/user_handler.go`

**Dependencies:** Task 1, Task 2

Implement `scim.ResourceHandler` on a struct that holds `*users.Manager` and `*config.Config`.

#### Helper functions needed:

`domainUserToResource(user *domain.User) scim.Resource` — converts domain user to SCIM resource:
- `Resource.ID` = `string(user.Identifier)`
- `Resource.ExternalID` = `optional.NewString(user.ExternalId)` if non-empty
- `Resource.Meta.Created` = `&user.CreatedAt`
- `Resource.Meta.LastModified` = `&user.UpdatedAt`
- `Resource.Attributes["userName"]` = `string(user.Identifier)`
- `Resource.Attributes["active"]` = `user.Disabled == nil`
- `Resource.Attributes["displayName"]` = `user.DisplayName()`
- `Resource.Attributes["name"]` = `map[string]interface{}{"givenName": user.Firstname, "familyName": user.Lastname}`
- `Resource.Attributes["emails"]` = `[]map[string]interface{}{{"value": user.Email, "primary": true}}`
- `Resource.Attributes["phoneNumbers"]` = phone if non-empty

`attributesToDomainUser(attrs scim.ResourceAttributes) *domain.User` — converts SCIM attributes to domain user:
- `userName` → `Identifier`
- `externalId` → `ExternalId`
- `name.givenName` → `Firstname`
- `name.familyName` → `Lastname`
- `emails[0].value` → `Email` (find primary first, fallback to first)
- `phoneNumbers[0].value` → `Phone`
- `active` == false → set `Disabled` to `time.Now()`

#### Handler methods:

**Create:** Convert attributes → `domain.User`, call `users.Manager.CreateUser(ctx, user)`, return as `scim.Resource`. On duplicate, return `errors.ScimError{Status: 409, Detail: "uniqueness"}`.

**Get:** Call `users.Manager.GetUser(ctx, UserIdentifier(id))`. If `domain.ErrNotFound`, return `errors.ScimErrorResourceNotFound(id)`. Convert to `scim.Resource`.

**GetAll:** Call `users.Manager.GetAllUsers(ctx)`. For each user, convert to `ResourceAttributes` map. If `params.FilterValidator != nil`, call `params.FilterValidator.PassesFilter(attrs)` and skip non-matching. Apply pagination using `params.StartIndex` and `params.Count`. Return `scim.Page{TotalResults: total, Resources: pageSlice}`.

**Replace:** Convert attributes → `domain.User`, set `Identifier = UserIdentifier(id)`, call `users.Manager.UpdateUser(ctx, user)`. Return updated resource.

**Patch:** Load existing user via `Get`. For each `PatchOperation`:
- `op:"replace"`, path `"active"`, value `false` → set `user.Disabled = &now`
- `op:"replace"`, path `"active"`, value `true` → set `user.Disabled = nil`
- `op:"replace"`, no path, value is map → apply each key (userName, name.givenName, etc.)
- `op:"add"` — same as replace for single-valued attributes
- `op:"remove"` — clear the targeted field
Call `users.Manager.UpdateUser(ctx, user)`. Return updated resource.

**Delete:** Behavior controlled by `cfg.Scim.DeleteAction`:
- `"disable"` (default): Load user, set `Disabled = &now`, `DisabledReason = "SCIM deprovisioned"`, call `users.Manager.UpdateUser(ctx, user)`. Return 204.
- `"delete"`: Call `users.Manager.DeleteUser(ctx, UserIdentifier(id))`. Return 204.

In both cases, if `domain.ErrNotFound`, return `errors.ScimErrorResourceNotFound(id)`.

---

### Task 4: Mount SCIM Handler + Wire in main.go

**Files:** `internal/app/api/core/server.go`, `cmd/wg-portal/main.go`

**Dependencies:** Task 2, Task 3

1. Modify `core.NewServer` to accept an optional `http.Handler` for SCIM:
   - Add parameter: `scimHandler http.Handler` (or pass via options/config)
   - Mount before `setupRoutes`: `s.root.Handle("/scim/", http.StripPrefix(basePath+"/scim", scimHandler))`
   - Only mount if handler is non-nil

2. In `cmd/wg-portal/main.go`, after `userManager` is created:
```go
var scimHandler http.Handler
if cfg.Scim.Enabled {
    scimHandler, err = scimapi.NewScimHandler(cfg, userManager)
    internal.AssertNoError(err)
}
```
Pass `scimHandler` to `core.NewServer`.

3. SCIM endpoints will be available at: `{basePath}/scim/v2/Users`, `{basePath}/scim/v2/ServiceProviderConfig`, etc.

---

### Task 5: Tests

**File:** `internal/app/api/scim/user_handler_test.go`

**Dependencies:** Task 3

Required by PR checklist: "Changes have reasonable test coverage" + "Tests pass with `make test`".

`make test` runs `go vet` + `go test -race -short`. Follow the project's existing mock-based
patterns (see `internal/app/wireguard/wireguard_peers_test.go`).

1. Mock the user manager interface used by the handler (the subset of `users.Manager` methods:
   `GetUser`, `GetAllUsers`, `CreateUser`, `UpdateUser`, `DeleteUser`).

2. Test bearer token middleware:
   - No header → 401
   - `Authorization: Bearer wrong` → 401
   - `Authorization: Basic user:pass` → 401
   - Valid bearer → passes through

3. Test handler via `scim.Server.ServeHTTP` with `httptest` (same pattern as the library's
   own `handlers_test.go`):
   - `POST /Users` valid → 201, response has `id`, `userName`, `meta`
   - `POST /Users` missing `userName` → 400
   - `GET /Users/{id}` exists → 200
   - `GET /Users/{id}` missing → 404
   - `GET /Users` → 200 with `totalResults`
   - `GET /Users?filter=userName eq "x"` → filtered results
   - `PUT /Users/{id}` → 200
   - `PATCH /Users/{id}` with `active: false` → user disabled
   - `DELETE /Users/{id}` with `delete_action: disable` → 204, user disabled not deleted
   - `DELETE /Users/{id}` with `delete_action: delete` → 204, user removed
   - `DELETE /Users/missing` → 404

4. Test `domainUserToResource` / `attributesToDomainUser` helpers:
   - Round-trip preserves key fields
   - `active: false` ↔ `Disabled != nil`
   - Multi-valued `emails` picks primary

---

### Task 6: Helm Docs

**Files:** `deploy/helm/values.yaml`, `deploy/helm/README.md`

**Dependencies:** Task 1

Required by PR checklist: "Helm docs are up-to-date with `make helm-docs`".

1. Add to `deploy/helm/values.yaml` after the `web: {}` line:
```yaml
  # -- (tpl/object) [SCIM configuration](https://wgportal.org/latest/documentation/configuration/overview/#scim) options.
  # Enables SCIM v2.0 provisioning endpoint for external identity providers.
  scim: {}
```

2. Run `make helm-docs` to regenerate `deploy/helm/README.md`.

---

## PR Checklist Mapping

From `.github/pull_request_template.md`:

| Checklist item | Covered by |
|---|---|
| Commits signed with `--signoff` | Developer workflow |
| Reasonable test coverage | Task 5 |
| Tests pass with `make test` | Task 5 |
| Helm docs up-to-date | Task 6 |

---

## Configuration Example

```yaml
scim:
  enabled: true
  bearer_token: "your-secret-scim-token-here"
  delete_action: "disable"  # "disable" (default) or "delete"
```

## Testing with curl

```bash
# Service provider config
curl -H "Authorization: Bearer your-secret-scim-token-here" \
  http://localhost:8888/scim/v2/ServiceProviderConfig

# List users
curl -H "Authorization: Bearer your-secret-scim-token-here" \
  http://localhost:8888/scim/v2/Users

# Get user by ID
curl -H "Authorization: Bearer your-secret-scim-token-here" \
  http://localhost:8888/scim/v2/Users/admin

# Create user
curl -X POST -H "Authorization: Bearer your-secret-scim-token-here" \
  -H "Content-Type: application/scim+json" \
  -d '{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"jdoe","name":{"givenName":"John","familyName":"Doe"},"emails":[{"value":"jdoe@example.com","primary":true}],"active":true}' \
  http://localhost:8888/scim/v2/Users

# Deactivate user (PATCH)
curl -X PATCH -H "Authorization: Bearer your-secret-scim-token-here" \
  -H "Content-Type: application/scim+json" \
  -d '{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"replace","value":{"active":false}}]}' \
  http://localhost:8888/scim/v2/Users/jdoe

# Delete user
curl -X DELETE -H "Authorization: Bearer your-secret-scim-token-here" \
  http://localhost:8888/scim/v2/Users/jdoe
```

## Not In Scope (library does not support)

- Bulk operations
- Sorting
- ETags (library sets them if you populate `Meta.Version`, but no conditional request handling)
- `/Me` endpoint (returns 501 automatically)
- Groups resource type (can be added later as a separate `ResourceHandler`)
