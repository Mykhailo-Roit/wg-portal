# SCIM 2.0 Deprovisioning — Implementation Plan

## Problem Statement

WG-Portal has no automatic mechanism to disable OIDC/OAuth users when they are removed or disabled in the identity provider (e.g., Entra ID, Okta, Google Workspace). LDAP has background sync with `disable_missing`, but OIDC/OAuth users remain active until their session expires. SCIM solves this by letting the IdP **push** deprovisioning events directly to WG-Portal.

See `oidc-user-disable-analysis.md` for the full gap analysis.

## Requirements

- **Deprovisioning-focused** SCIM endpoint: handle `PATCH active=false` and `DELETE /Users/{id}`
- Must work with **any SCIM-compliant IdP** (Entra ID, Okta, Google Workspace, etc.), not just Entra
- **First iteration**: static bearer token auth. Architecture allows future OAuth2 client credentials grant
- Deprovisioned users get their **WireGuard peers disabled** (same behavior as LDAP `disable_missing`)
- Add `scim` config sub-section under each OIDC/OAuth provider
- Use the provider's existing **`field_map`** to resolve SCIM attributes to WG-Portal user identifiers
- Store SCIM protocol documentation in the project root

## Background — field_map Resolution

The `field_map.user_identifier` config (default: `sub`) determines what JWT claim becomes the `domain.UserIdentifier` in WG-Portal. When SCIM sends a request, we need to map SCIM attributes to WG-Portal users using the same logic:

- The **bearer token** identifies which provider config to use
- From that provider config, we get the **`field_map`**
- If `field_map.user_identifier` = `sub` → SCIM `externalId` is the lookup key
- If `field_map.user_identifier` = `email` → SCIM `userName` or `emails[0].value` is the lookup key
- The SCIM service supports a `scim.user_identifier_mapping` override for cases where the SCIM attribute names differ from the JWT claim names

### Flow Diagram

```
IdP sends SCIM request
  │
  ▼
SCIM Auth Middleware
  │ Bearer Token → match against provider configs
  ▼
Resolve provider_name + field_map
  │
  ▼
SCIM Handler
  │ Map SCIM userName/externalId using field_map
  ▼
Lookup user in DB by UserIdentifier
  │
  ▼ (PATCH active=false or DELETE)
Disable user + publish TopicUserDisabled
  │
  ▼
WireGuard Peers Disabled
```

## Existing Patterns to Follow

- **LDAP sync** (`internal/app/users/ldap_sync.go`): `disableMissingLdapUsers` sets `Disabled` + `DisabledReason` and publishes `TopicUserDisabled` which cascades to disable WireGuard peers
- **v1 API** (`internal/app/api/v1/`): uses `routegroup.Bundle`, `net/http` handlers, `core.ApiEndpointSetupFunc` pattern
- **OAuth field mapping** (`internal/app/auth/oauth_common.go`): `parseOauthUserInfo` + `getOauthFieldMapping` resolve JWT claims to `domain.UserIdentifier` using `field_map`
- **Event bus** (`internal/app/eventbus.go`): `TopicUserDisabled` triggers peer disablement

## Task Breakdown

### Task 1: SCIM Protocol Documentation

**Objective:** Create `SCIM_PROTOCOL.md` in the project root.

**Content:**
- Protocol overview (RFC 7643/7644)
- Supported endpoints and their behavior
- Request/response examples for:
  - Deprovisioning: `PATCH active=false`, `DELETE`
  - User query: `GET` with filters
  - User creation: `POST`
  - Schema discovery: `GET /Schemas`, `/ServiceProviderConfig`, `/ResourceTypes`
- Authentication: bearer token
- How `field_map` maps SCIM attributes to WG-Portal users
- IdP setup instructions (Entra ID, Okta, generic)

**Demo:** Documentation file exists and is readable.

---

### Task 2: SCIM Configuration

**Objective:** Add SCIM config to `internal/config/auth.go`.

**Changes:**

1. Add `ScimConfig` struct:
   ```go
   type ScimConfig struct {
       Enabled               bool   `yaml:"enabled"`
       BearerToken           string `yaml:"bearer_token"`
       DisableMissing        bool   `yaml:"disable_missing"`
       AutoReEnable          bool   `yaml:"auto_re_enable"`
       UserIdentifierMapping string `yaml:"user_identifier_mapping"` // "userName" or "externalId"; auto-detect from field_map if empty
   }
   ```

2. Add `Scim ScimConfig` field to both `OpenIDConnectProvider` and `OAuthProvider` in `internal/config/auth.go`

3. Add to `internal/domain/base.go`:
   ```go
   DisabledReasonScimMissing = "missing in scim provider"
   ```

4. Add to `internal/domain/context.go`:
   ```go
   CtxSystemScimProvisioner = "_WG_SYS_SCIM_"
   ```
   Plus a `ScimContextUserInfo()` helper (same pattern as `LdapSyncContextUserInfo()`).

5. Update `config.yml.sample` with example SCIM config under an OIDC provider:
   ```yaml
   oidc:
     - id: entraid
       # ... existing fields ...
       scim:
         enabled: true
         bearer_token: "your-secret-scim-token"
         disable_missing: true
         auto_re_enable: true
   ```

**Test:** Config parsing loads SCIM fields correctly.

---

### Task 3: SCIM Bearer Token Authentication Middleware

**Objective:** Create `internal/app/api/scim/middleware.go`.

**Behavior:**
- Extract `Authorization: Bearer <token>` header
- Iterate all OIDC/OAuth providers with `scim.enabled: true`
- Match the token against each provider's `scim.bearer_token`
- On match: store the matched provider config (including `field_map` and `provider_name`) in the request context
- Set system admin context (`ScimContextUserInfo()`) — SCIM operations are always admin-level
- On failure: return `401` with SCIM error schema:
  ```json
  {
    "schemas": ["urn:ietf:params:scim:api:messages:2.0:Error"],
    "status": "401",
    "detail": "invalid or missing bearer token"
  }
  ```

**Context key for provider config:** Define a context key so handlers can retrieve the resolved provider config:
```go
type scimProviderContextKey struct{}

func GetScimProvider(ctx context.Context) (*ScimProviderInfo, bool)
func SetScimProvider(ctx context.Context, info *ScimProviderInfo) context.Context
```

Where `ScimProviderInfo` contains:
```go
type ScimProviderInfo struct {
    ProviderName string
    FieldMap     config.OauthFields
    ScimConfig   config.ScimConfig
}
```

**Test:** Rejects requests without token, accepts valid token, resolves correct provider config.

---

### Task 4: SCIM User Model and Response Helpers

**Objective:** Create `internal/app/api/scim/models.go`.

**Types:**
```go
type ScimUser struct {
    Schemas    []string       `json:"schemas"`
    ID         string         `json:"id"`
    ExternalID string         `json:"externalId,omitempty"`
    UserName   string         `json:"userName"`
    Name       *ScimName      `json:"name,omitempty"`
    Emails     []ScimEmail    `json:"emails,omitempty"`
    Active     bool           `json:"active"`
    Meta       *ScimMeta      `json:"meta,omitempty"`
}

type ScimListResponse struct {
    Schemas      []string  `json:"schemas"`
    TotalResults int       `json:"totalResults"`
    StartIndex   int       `json:"startIndex"`
    ItemsPerPage int       `json:"itemsPerPage"`
    Resources    []ScimUser `json:"Resources"`
}

type ScimPatchOp struct {
    Schemas    []string        `json:"schemas"`
    Operations []ScimOperation `json:"Operations"`
}

type ScimOperation struct {
    Op    string `json:"op"`
    Path  string `json:"path,omitempty"`
    Value any    `json:"value,omitempty"`
}

type ScimError struct {
    Schemas []string `json:"schemas"`
    Status  string   `json:"status"`
    Detail  string   `json:"detail,omitempty"`
}
```

**Key functions:**

1. `DomainUserToScimUser(user *domain.User, fieldMap config.OauthFields) ScimUser` — maps WG-Portal user to SCIM representation using the provider's `field_map`:
   - `ScimUser.ID` = `string(user.Identifier)` (always the internal ID)
   - If `field_map.user_identifier` = `email` → `ScimUser.UserName` = `user.Email`, `ScimUser.ExternalID` = `string(user.Identifier)`
   - If `field_map.user_identifier` = `sub` (default) → `ScimUser.UserName` = `user.Email`, `ScimUser.ExternalID` = `string(user.Identifier)`

2. `ResolveUserIdentifier(scimUser ScimUser, providerInfo ScimProviderInfo) domain.UserIdentifier` — given a SCIM user payload and the provider's field_map, extract the WG-Portal `UserIdentifier`:
   - If `scim.user_identifier_mapping` = `"userName"` → use `scimUser.UserName`
   - If `scim.user_identifier_mapping` = `"externalId"` → use `scimUser.ExternalID`
   - Else auto-detect: if `field_map.user_identifier` = `email` → use `scimUser.UserName`
   - Else (default `sub`) → use `scimUser.ExternalID`

3. `ParseScimFilter(filter string) (attr, op, value string, err error)` — parse SCIM filter for `eq` operator on `userName` and `externalId` (what Entra ID uses).

**Test:** Conversion preserves data; `ResolveUserIdentifier` correctly maps based on different `field_map` configs; filter parser handles `userName eq "test@example.com"`.

---

### Task 5: SCIM Service Layer

**Objective:** Create `internal/app/api/scim/service.go`.

**Dependencies:**
```go
type ScimService struct {
    cfg   *config.Auth
    users UserDatabaseRepo  // same interface as users.UserDatabaseRepo
    bus   EventBus
}
```

**Methods:**

- `GetUser(ctx, scimId string, provider ScimProviderInfo) (*domain.User, error)` — look up user by `UserIdentifier`. The `scimId` is the SCIM resource `id` which maps directly to `domain.UserIdentifier`.

- `QueryUsers(ctx, filter string, startIndex, count int, provider ScimProviderInfo) ([]domain.User, int, error)` — list/filter users scoped to the provider (only users with `UserSourceOauth` and matching `provider_name`). Apply SCIM filter using `field_map` to map filter attribute to the correct DB field:
  - `userName eq "x"` → if `field_map.user_identifier` = `email`, search by `UserIdentifier`; else search by `Email`
  - `externalId eq "x"` → search by `UserIdentifier` (since `externalId` maps to the IdP's subject)

- `CreateUser(ctx, scimUser ScimUser, provider ScimProviderInfo) (*domain.User, error)` — create user with `UserSourceOauth`, provider's `provider_name`, and map SCIM fields to domain fields using `field_map`. Use `ResolveUserIdentifier` to determine the `UserIdentifier`.

- `PatchUser(ctx, scimId string, ops []ScimOperation, provider ScimProviderInfo) (*domain.User, error)`:
  - When `active` is set to `false`: disable user with `DisabledReasonScimMissing`, publish `TopicUserDisabled`
  - When `active` is set to `true` and `AutoReEnable` is configured and `DisabledReason` is `DisabledReasonScimMissing`: re-enable user, publish `TopicUserEnabled`
  - Other attribute updates: update user fields using `field_map` mapping

- `DeleteUser(ctx, scimId string, provider ScimProviderInfo) error` — soft-disable with `DisabledReasonScimMissing`, publish `TopicUserDisabled`

**Test:** Disabling user sets `Disabled` field and publishes event; user lookup works with both `email` and `sub` based `field_map` configs.

---

### Task 6: SCIM HTTP Handlers and Route Registration

**Objective:** Create `internal/app/api/scim/handlers.go`.

**Endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/scim/v2/Users` | List/filter users |
| `GET` | `/api/scim/v2/Users/{id}` | Get single user |
| `POST` | `/api/scim/v2/Users` | Create user |
| `PATCH` | `/api/scim/v2/Users/{id}` | Partial update (deprovisioning) |
| `DELETE` | `/api/scim/v2/Users/{id}` | Soft-delete user |
| `GET` | `/api/scim/v2/Schemas` | Supported schemas |
| `GET` | `/api/scim/v2/ServiceProviderConfig` | SCIM capabilities |
| `GET` | `/api/scim/v2/ResourceTypes` | Resource type metadata |

**Behavior:**
- Handlers retrieve the provider config from request context (set by middleware in Task 3)
- Pass provider config to service layer for `field_map`-aware user resolution
- All responses use `Content-Type: application/scim+json`
- Create `NewScimApi(cfg *config.Auth, service *ScimService)` returning `core.ApiEndpointSetupFunc`

**Test:** HTTP requests verify correct status codes, response formats, and field_map-based user resolution.

---

### Task 7: Wire SCIM into main.go and Startup

**Objective:** Integrate SCIM API into `cmd/wg-portal/main.go`.

**Changes:**
1. After creating `userManager`, check if any OIDC/OAuth provider has `scim.enabled: true`
2. If yes, instantiate:
   ```go
   scimService := scim.NewScimService(&cfg.Auth, database, eventBus)
   scimApi := scim.NewScimApi(&cfg.Auth, scimService)
   ```
3. Register alongside existing APIs:
   ```go
   webSrv, err := core.NewServer(cfg, apiFrontend, apiV1, scimApi)
   ```
4. SCIM mounts at `/api/scim/v2` with its own bearer token auth middleware (not session-based, not Basic Auth)

**Test:** App starts with SCIM enabled; endpoints accessible.

**Demo — Full end-to-end:**
1. Configure SCIM in `config.yml` with `field_map.user_identifier: email`
2. Start the app
3. Send SCIM `PATCH /api/scim/v2/Users/{id}` with `active=false` and bearer token
4. Verify user is disabled in DB with `DisabledReason = "missing in scim provider"`
5. Verify user's WireGuard peers are disabled

## File Structure (New Files)

```
SCIM_PROTOCOL.md                              # Task 1: Protocol documentation
SCIM_IMPLEMENTATION_PLAN.md                   # This file
internal/
  domain/
    base.go                                   # Task 2: Add DisabledReasonScimMissing
    context.go                                # Task 2: Add CtxSystemScimProvisioner + ScimContextUserInfo()
  config/
    auth.go                                   # Task 2: Add ScimConfig, add Scim field to OIDC/OAuth providers
  app/
    api/
      scim/
        middleware.go                         # Task 3: Bearer token auth + provider resolution
        models.go                             # Task 4: SCIM types + field_map conversion
        service.go                            # Task 5: Business logic
        handlers.go                           # Task 6: HTTP handlers + route registration
config.yml.sample                             # Task 2: Add SCIM example config
cmd/wg-portal/main.go                        # Task 7: Wire SCIM into startup
```

## Modified Files

- `internal/config/auth.go` — add `ScimConfig` struct, add `Scim` field to `OpenIDConnectProvider` and `OAuthProvider`
- `internal/domain/base.go` — add `DisabledReasonScimMissing` constant
- `internal/domain/context.go` — add `CtxSystemScimProvisioner` and `ScimContextUserInfo()`
- `config.yml.sample` — add SCIM example config
- `cmd/wg-portal/main.go` — wire SCIM API into server startup
- `docs/documentation/configuration/overview.md` — document SCIM config fields under OIDC/OAuth sections
- `docs/documentation/configuration/examples.md` — add SCIM config example
- `docs/documentation/usage/user-sync.md` — document SCIM-based deprovisioning alongside LDAP sync
- `docs/documentation/usage/authentication.md` — mention SCIM provisioning as an option for OIDC/OAuth providers
- `deploy/helm/values.yaml` — no changes needed (auth config is passed as raw object)

---

## PR Checklist Tasks

These tasks correspond to the `.github/pull_request_template.md` checklist and must be completed before the PR is ready.

### Task 8: Tests

**Objective:** Ensure all new code has reasonable test coverage and all tests pass.

**Unit tests to write:**
- `internal/app/api/scim/models_test.go`:
  - `ResolveUserIdentifier` with `field_map.user_identifier=email` → uses `userName`
  - `ResolveUserIdentifier` with `field_map.user_identifier=sub` (default) → uses `externalId`
  - `ResolveUserIdentifier` with explicit `scim.user_identifier_mapping` override
  - `DomainUserToScimUser` round-trip preserves data
  - `ParseScimFilter` parses `userName eq "test@example.com"` correctly
  - `ParseScimFilter` rejects invalid filters
- `internal/app/api/scim/middleware_test.go`:
  - Rejects request without `Authorization` header → 401
  - Rejects request with invalid token → 401
  - Accepts request with valid token → sets provider context
  - Matches correct provider when multiple providers have SCIM enabled
- `internal/app/api/scim/service_test.go`:
  - `PatchUser` with `active=false` sets `Disabled` and `DisabledReason=DisabledReasonScimMissing`
  - `PatchUser` with `active=true` + `AutoReEnable` + reason is `DisabledReasonScimMissing` → re-enables
  - `PatchUser` with `active=true` + reason is NOT `DisabledReasonScimMissing` → does not re-enable
  - `DeleteUser` soft-disables with correct reason

**Verification:**
```bash
make test
```
All existing tests must continue to pass. New tests must pass.

### Task 9: Signed Commits

**Objective:** All commits are signed with `git commit --signoff`.

Every commit in the PR branch must include the `Signed-off-by` trailer. Use:
```bash
git commit --signoff -m "feat: add SCIM 2.0 deprovisioning endpoint"
```

### Task 10: Helm Docs

**Objective:** Helm chart documentation is up-to-date.

Since the SCIM config flows through the existing `config.auth` raw object in `values.yaml`, no helm template changes are needed. However, verify docs are current:
```bash
make helm-docs
```
If the command produces any diff, commit the updated docs.

### Task 11: Project Documentation (MkDocs)

**Objective:** Update the MkDocs-based documentation at `docs/`.

**Files to update:**

1. `docs/documentation/configuration/overview.md` — add SCIM config fields documentation under the OIDC and OAuth sections:
   - `scim.enabled` — Enable SCIM provisioning endpoint for this provider
   - `scim.bearer_token` — Static bearer token for SCIM authentication
   - `scim.disable_missing` — Disable users when deprovisioned via SCIM
   - `scim.auto_re_enable` — Re-enable users when re-provisioned via SCIM
   - `scim.user_identifier_mapping` — Override SCIM-to-UserIdentifier mapping

2. `docs/documentation/configuration/examples.md` — add a complete SCIM configuration example (Entra ID + OIDC + SCIM)

3. `docs/documentation/usage/user-sync.md` — add a SCIM section explaining:
   - How SCIM deprovisioning works (push-based vs LDAP's pull-based)
   - That `disable_missing` controls whether deprovisioned users are disabled
   - That WireGuard peers are disabled when the user is disabled (same as LDAP)

4. `docs/documentation/usage/authentication.md` — mention SCIM as an option for automated user lifecycle management with OIDC/OAuth providers
