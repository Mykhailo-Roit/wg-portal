# Changelog

## Unreleased

### Added
- Startup validation for `web.external_url` to reject relative URLs, path components, query strings, fragments, and public `http` endpoints.
- Machine-readable authentication error identifiers on v0 auth endpoints via `ErrorId`.
- Unit tests for auth provider display name sanitization, `external_url` validation, and auth error responses.

### Changed
- Login provider names are now rendered as plain text in the web UI instead of HTML.
- OIDC and OAuth provider `display_name` values are sanitized during config loading and fall back to `provider_name` when blank.
- Plain OAuth configurations now emit startup warnings to make OIDC the preferred configuration path.
- Authentication endpoints now return safer user-facing error messages while keeping detailed causes in backend logs.
