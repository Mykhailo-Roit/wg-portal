# OIDC/OAuth (EntraID) — No Background Sync Exists

Unlike LDAP, there is **no background synchronization service** for OIDC/OAuth providers (including EntraID). The codebase only has a background sync loop for LDAP (`runLdapSynchronizationService`). The OIDC/OAuth providers have no equivalent — no `DisableMissing`, `SyncInterval`, or `AutoReEnable` config options exist in `OpenIDConnectProvider` or `OAuthProvider` structs.

## How OIDC/OAuth user state is checked

The only mechanism is **login-time and request-time validation**:

1. **At login** (`OauthLoginStep2` in `auth.go:502-565`): When a user authenticates via EntraID, the portal exchanges the code for a token, fetches user info, and calls `processUserInfo`. If the user already exists in the DB, it calls `updateExternalUser` which updates profile fields (email, name, etc.) but does **not** disable users. If the user is already disabled/locked, it simply skips the update and the login is rejected at line 543: `if user.IsLocked() || user.IsDisabled()`.

2. **On every authenticated API request** (`LoggedIn` middleware in `web_authentication.go:56`): The middleware calls `IsUserValid()` which checks the DB for the `Disabled` and `Locked` fields. If a user was manually disabled by an admin, their session is immediately invalidated.

## What this means for EntraID

- If a user is **removed or disabled in EntraID**, WireGuard Portal **will not automatically detect this**. There is no periodic poll of the OIDC provider.
- The user simply **won't be able to log in again** because the OAuth flow will fail at the EntraID side (token exchange or user info fetch will fail).
- However, their **existing session remains valid** until it expires or they make a request that triggers `IsUserValid()` — which checks the local DB, not EntraID.
- Their **WireGuard peers remain active** since no `TopicUserDisabled` event is published.

## LDAP comparison (for reference)

LDAP has a full background sync with these config options:
- `sync_interval` — how often to poll (e.g., `5m`)
- `disable_missing` — auto-disable users not found in LDAP
- `auto_re_enable` — re-enable users that reappear in LDAP

The sync runs in `runLdapSynchronizationService` on a ticker, calling `disableMissingLdapUsers` which iterates all DB users, checks if they exist in LDAP, and sets `Disabled` + publishes `TopicUserDisabled` (which cascades to disable all WireGuard peers).

## Bottom line

**For EntraID/OIDC: there is no automatic sync or auto-disable.** If you need users to be automatically disabled when removed from EntraID, your options are:

1. **Use LDAP sync alongside OIDC** — if your EntraID supports LDAP (Azure AD DS), configure an LDAP provider with `disable_missing: true` and a `sync_interval`.
2. **Use the REST API** — build an external script/webhook that queries Microsoft Graph API for disabled/deleted users and calls the WG Portal API to disable them.
3. **Rely on session expiry** — users can't re-authenticate, but existing sessions and peers stay active until manually cleaned up.

---

## Can we implement OIDC sync similar to LDAP?

Yes, but it requires new code. The challenge is fundamentally different from LDAP.

### Why it doesn't exist yet

LDAP sync works by **querying the LDAP directory directly** — it connects, runs a search filter, gets all users, and compares against the local DB. This is a pull-based model that LDAP natively supports.

OIDC/OAuth **has no equivalent "list all users" endpoint**. The OIDC spec only defines authentication flows and a `/userinfo` endpoint for the currently authenticated user. There's no standard way to enumerate all users or check if a specific user still exists.

### How to implement it for EntraID

You'd need to use the **Microsoft Graph API** (not OIDC) to query user status. The implementation would follow the same pattern as `ldap_sync.go`:

#### What's needed:

1. **Config additions** to `OpenIDConnectProvider` in `internal/config/auth.go`:
   - `sync_interval` (like LDAP's)
   - `disable_missing` (like LDAP's)
   - `auto_re_enable` (like LDAP's)
   - `graph_api_url` or `tenant_id` for Microsoft Graph access

2. **New file** `internal/app/users/oidc_sync.go` mirroring `ldap_sync.go`:
   - `runOidcSynchronizationService` — periodic ticker loop (same pattern as `runLdapSynchronizationService`)
   - `synchronizeOidcUsers` — calls Microsoft Graph API `GET /users?$filter=accountEnabled eq true` to get active users
   - `disableMissingOidcUsers` — iterates all DB users with `UserSourceOauth` auth source, disables those not found in Graph response

3. **New disabled reason constant** in `internal/domain/base.go`:
   - `DisabledReasonOauthMissing = "missing in oauth provider"`

4. **Hook it up** in `user_manager.go` `StartBackgroundJobs`:
   ```go
   func (m Manager) StartBackgroundJobs(ctx context.Context) {
       go m.runLdapSynchronizationService(ctx)
       go m.runOidcSynchronizationService(ctx)  // new
   }
   ```

### The key challenge

Microsoft Graph API requires a **separate OAuth client credentials flow** (app-only token) to list/check users. This is different from the user-facing OIDC flow already configured. You'd need:
- An Azure App Registration with `User.Read.All` application permission (not delegated)
- A client secret or certificate for the app-only auth
- The tenant ID

The existing `client_id`/`client_secret` in the OIDC config *might* work if the app registration has the right permissions, but typically the OIDC config uses delegated permissions while Graph API sync needs application permissions.

### Simpler alternative: external script

Instead of modifying wg-portal source, you could write an external cron job that:
1. Queries Microsoft Graph for disabled/deleted users
2. Calls the WG Portal REST API to disable those users

This avoids forking the project and achieves the same result.
