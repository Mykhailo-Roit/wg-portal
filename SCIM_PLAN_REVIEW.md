# SCIM Implementation Plan — Architectural Review

Review of `SCIM_IMPLEMENTATION_PLAN.md` against the existing wg-portal codebase.

---

## 1. Route Registration Architecture Mismatch

`setupRoutes` mounts API versions at `/api/{version}` and sets up RapiDoc swagger endpoints for every version. The plan wants `/api/scim/v2`, which means `ApiVersion` would be `"scim/v2"` — a hack that breaks the assumption that versions are simple identifiers like `"v0"`, `"v1"`. SCIM doesn't need swagger docs, and the SCIM bearer auth middleware needs to be applied inside the group, not at the server level.

**Recommendation:** Either add a separate `SetupRawEndpoints` path in `Server` that mounts without the swagger/version boilerplate, or mount SCIM routes directly on `s.root` bypassing `setupRoutes` entirely (add a `MountRaw(path, handler)` method to `Server`).

## 2. No Query-by-Provider in the Database Layer

`QueryUsers` needs to filter users by `UserSourceOauth` + `provider_name`. The current `UserDatabaseRepo` interface has no such method — only `GetAllUsers()` and `FindUsers(search string)`. The LDAP sync works around this by calling `GetAllUsers()` and filtering in Go.

For SCIM this is a problem: `GetAllUsers()` loads every user into memory. Fine for LDAP sync running every 5 minutes, but SCIM `GET /Users` is an HTTP endpoint that could be called frequently. SCIM pagination (`startIndex`, `count`) can't be implemented efficiently if you load all users first.

**Recommendation:** Add a scoped query method to the database repo:
```go
GetUsersByAuthSource(ctx, source UserSource, providerName string, offset, limit int) ([]User, int, error)
```
Or accept that the first iteration loads all users and filters in-memory, but document this as a known scalability limitation.

## 3. `ScimUser.ID` vs `UserIdentifier` Confusion

The plan says `ScimUser.ID` = `string(user.Identifier)` (always the internal ID), and `GetUser` looks up by `UserIdentifier` where `scimId` maps directly to `domain.UserIdentifier`.

This is correct but fragile. SCIM spec says the `id` is assigned by the service provider and is immutable. In WG-Portal, `UserIdentifier` is derived from the IdP claim (`sub` or `email`). If `field_map.user_identifier=email`, then `UserIdentifier` is an email address. The IdP (e.g., Entra ID) may use a different value as the SCIM `id` than what it sends as `externalId` or `userName`.

**Recommendation:** Add explicit documentation that the SCIM `id` returned in `POST` responses is what the IdP must use for subsequent `PATCH`/`DELETE`/`GET` calls. Consider whether a separate `scim_external_id` column is needed to decouple SCIM identity from WG-Portal's `UserIdentifier`.

## 4. Bearer Token in Plaintext Config — Security Issue

The plan stores `bearer_token` as a plain `string` in the config YAML. This token is effectively an admin-level credential (SCIM operations are admin-level per the plan). Anyone who can read the config file can disable any user.

**Recommendations:**
- Use the existing `PrivateString` type (from `domain/base.go`) so the token isn't leaked in JSON marshaling/logging
- Support environment variable substitution (check if the config loader already supports this — the LDAP `bind_pass` has the same concern)
- Add a `Sanitize()` method to `ScimConfig` that validates token length/entropy (minimum 32 chars)
- Log a warning at startup if the token is too short

## 5. Missing `ReEnablePeerAfterUserEnable` Interaction

The plan says: when `active` is set to `true` and `AutoReEnable` is configured, re-enable user and publish `TopicUserEnabled`.

But `TopicUserEnabled` triggers `handleUserEnabledEvent` which only re-enables peers if `cfg.Core.ReEnablePeerAfterUserEnable` is `true`. If an admin has `ReEnablePeerAfterUserEnable=false` globally but `auto_re_enable=true` in SCIM config, the user gets re-enabled but peers stay disabled.

**Recommendation:** Document this interaction explicitly. Consider whether SCIM `auto_re_enable` should override `ReEnablePeerAfterUserEnable` or respect it.

## 6. Race Condition: SCIM vs Login Flow

Not addressed in the plan. Scenario:
1. SCIM `PATCH active=false` disables a user
2. The user simultaneously logs in via OIDC (their IdP session is still valid)
3. `processUserInfo` in `oauth_common.go` calls `updateExternalUser` which updates the user but doesn't check `DisabledReason`

The LDAP sync doesn't have this problem because LDAP login checks the LDAP server directly. But OIDC login succeeds as long as the IdP grants a token — the IdP might have a propagation delay before the user is actually disabled there.

**Recommendation:** Verify that the existing `IsLocked() || IsDisabled()` check in `OauthLoginStep2` (line 543) is evaluated *after* the user update, not before. If it's before, the login will succeed and re-activate the session for a SCIM-disabled user.

## 7. No Idempotency Handling for `POST /Users`

SCIM spec (RFC 7644 §3.3) says if a `POST /Users` creates a user that already exists, the server should return `409 Conflict`. The plan's `CreateUser` doesn't mention this.

Entra ID and Okta both retry failed SCIM operations. Without proper `409` handling, you'll get duplicate creation attempts that either error out confusingly or silently succeed.

**Recommendation:** `CreateUser` should check if a user with the resolved `UserIdentifier` already exists and return `409` with a SCIM error response.

## 8. No Rate Limiting or Request Size Limits

SCIM endpoints are internet-facing (they must be, for IdPs to reach them). The plan has no mention of rate limiting, request body size limits, or maximum filter result count.

**Recommendation:** Add at minimum:
- Request body size limit (e.g., 1MB)
- Default `count` cap for `GET /Users` (e.g., 100)
- Consider rate limiting middleware (even basic, like 100 req/min per token)

## 9. `DELETE` Semantics: Soft-Disable vs What IdPs Expect

The plan says `DELETE /Users/{id}` does a soft-disable. SCIM spec says `DELETE` should return `204 No Content` and the resource should no longer be retrievable via `GET`. But if you soft-disable, the user is still returned by `GET /Users/{id}` (just with `active=false`).

Entra ID specifically checks this — after sending `DELETE`, it may send `GET` to verify the user is gone. If it gets a `200` back, it may retry the delete.

**Recommendation:** Either make `DELETE` return `204` and make `GET /Users/{id}` return `404` for disabled-by-SCIM users, or return the user with `active=false` but document that this deviates from strict SCIM compliance.

## 10. Missing `PUT /Users/{id}` Endpoint

SCIM spec requires `PUT` for full resource replacement. Entra ID uses `PUT` in some provisioning flows (not just `PATCH`). The plan only has `PATCH`.

**Recommendation:** Add `PUT /Users/{id}` that does a full attribute replacement. Even if the first iteration only cares about `active`, some IdPs send `PUT` instead of `PATCH` for deprovisioning.

## 11. `PersistLocalChanges` Not Respected

The LDAP sync explicitly checks `user.PersistLocalChanges` and skips users with that flag. The SCIM plan doesn't mention this at all. If an admin sets `PersistLocalChanges=true` on a user, SCIM should probably respect that and not disable them.

**Recommendation:** Add `PersistLocalChanges` check in `PatchUser` and `DeleteUser`, same as LDAP sync does.

## 12. No SCIM `ETag` / Concurrency Control

SCIM spec supports `ETag` headers for optimistic concurrency. Without it, if two SCIM operations arrive simultaneously for the same user, you get a last-write-wins race. The existing `SaveUser` uses a callback pattern that helps, but the SCIM layer doesn't leverage it properly.

Low priority for first iteration, but worth noting.

## 13. Service Layer Directly Accesses DB — Bypasses User Manager

**This is the most architecturally significant issue in the plan.**

The plan has `ScimService` depending on `UserDatabaseRepo` directly. But the existing codebase routes all user mutations through `users.Manager` which handles:
- Password hashing
- Event publishing (`TopicUserDisabled`, `TopicUserEnabled`, `TopicUserUpdated`)
- Validation (`validateModifications`)
- `CopyCalculatedAttributes`

By going directly to the DB repo, SCIM bypasses all of this. The LDAP sync also goes through `m.create()` and `m.update()` (Manager methods), not directly to the DB.

**Recommendation:** `ScimService` should depend on the user `Manager` (or an interface extracted from it), not `UserDatabaseRepo`. Use `Manager.update()` for disabling users so that event publishing and validation are consistent.

## 14. No Logging / Audit Trail

The plan doesn't mention audit logging. The existing codebase has `TopicAuditLoginSuccess`, `TopicAuditPeerChanged`, etc. SCIM operations (especially deprovisioning) are security-sensitive and should be audited.

**Recommendation:** Publish audit events for SCIM operations, at minimum for disable/delete actions.

## 15. Config Validation at Startup

No `Sanitize()` method is planned for `ScimConfig`. The LDAP provider has `Sanitize()` that validates config and sets defaults. Without it:
- Empty `bearer_token` with `enabled: true` silently creates an unauthenticated endpoint
- No validation that the referenced provider actually exists

**Recommendation:** Add `Sanitize()` to `ScimConfig` that:
- Rejects `enabled: true` with empty `bearer_token`
- Validates `user_identifier_mapping` is one of `""`, `"userName"`, `"externalId"`
- Warns if `disable_missing` is `false` (since that's the whole point of SCIM)

## 16. Multiple Providers with Same Bearer Token

The middleware iterates all providers and matches the first one with a matching token. If two providers accidentally have the same token, the wrong provider's `field_map` gets used silently.

**Recommendation:** Validate at startup that all SCIM bearer tokens are unique across providers.

---

## Priority Summary

| Priority | Issue | Impact |
|----------|-------|--------|
| Critical | #13 — Bypasses User Manager | Breaks event publishing, validation, consistency |
| Critical | #4 — Plaintext bearer token | Security |
| High | #1 — Route registration mismatch | Won't wire up cleanly |
| High | #2 — No efficient DB query | Performance at scale |
| High | #7 — No 409 Conflict on duplicate POST | IdP retry loops |
| High | #9 — DELETE semantics | IdP compatibility |
| High | #15 — No config validation | Silent misconfigurations |
| Medium | #6 — Race with login flow | Edge case but real |
| Medium | #10 — Missing PUT | Some IdPs need it |
| Medium | #11 — PersistLocalChanges ignored | Inconsistent with LDAP |
| Medium | #14 — No audit logging | Security compliance |
| Medium | #16 — Duplicate token detection | Silent misconfiguration |
| Low | #3 — ID confusion | Needs documentation |
| Low | #5 — ReEnablePeerAfterUserEnable interaction | Config confusion |
| Low | #8 — No rate limiting | Hardening |
| Low | #12 — No ETag | Concurrency edge case |
