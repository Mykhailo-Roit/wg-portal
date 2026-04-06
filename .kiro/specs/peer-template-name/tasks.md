# Implementation Plan: peer-template-name

## Overview

Implement the `peer_template_name` configuration feature by layering changes from config → domain → service → frontend. Each task builds on the previous, ending with all creation paths wired together and the frontend pre-filling the evaluated name automatically.

## Tasks

- [x] 1. Add `PeerTemplateName` to config
  - Add `PeerTemplateName string \`yaml:"peer_template_name"\`` to the `Core` struct in `internal/config/config.go`
  - Set the default in `defaultConfig()`: `cfg.Core.PeerTemplateName = getEnvStr("WG_PORTAL_CORE_PEER_TEMPLATE_NAME", "Peer {{.Random}}")`
  - _Requirements: 1.1, 1.2, 6.1, 6.2, 6.3_

  - [x] 1.1 Write unit test for default config value
    - In `internal/config/config_test.go` (create if absent), assert `defaultConfig().Core.PeerTemplateName == "Peer {{.Random}}"`
    - _Requirements: 1.2, 6.1_

- [x] 2. Implement `PeerNameTemplateData`, `generateRandomString`, and `ApplyPeerNameTemplate` in `internal/domain/peer.go`
  - Add `PeerNameTemplateData` struct with fields `Id`, `Random`, `Email`, `Firstname`, `Lastname`, `PeerName` (all `string`)
  - Add private `generateRandomString(n int) string` using `crypto/rand` with charset `A-Za-z0-9`
  - Add `ApplyPeerNameTemplate(tmpl string, data PeerNameTemplateData) (string, error)` using `text/template`
  - _Requirements: 1.3, 1.4, 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7_

  - [x] 2.1 Write unit tests for `ApplyPeerNameTemplate`
    - `TestApplyPeerNameTemplate_Variables`: each variable resolves to the correct field value for a known input
    - `TestApplyPeerNameTemplate_InvalidTemplate`: invalid template string returns non-nil error
    - `TestApplyPeerNameTemplate_EmptyUserFields`: nil/empty user fields produce empty string substitutions
    - _Requirements: 1.3, 1.4, 4.1–4.5_

  - [x] 2.2 Write property test: Property 1 — Template variable resolution
    - **Property 1: Template variable resolution**
    - **Validates: Requirements 1.3, 1.4, 4.1, 4.2, 4.3, 4.4, 4.5**
    - Using `rapid`, generate random `PeerNameTemplateData` values; for each field construct a single-variable template and assert output equals the field value

  - [x] 2.3 Write property test: Property 2 — Random variable is 8-char alphanumeric
    - **Property 2: Random variable is 8-char alphanumeric**
    - **Validates: Requirements 4.6**
    - Using `rapid`, evaluate `"{{.Random}}"` and assert `len(result) == 8` and all chars match `[A-Za-z0-9]`

  - [x] 2.4 Write property test: Property 3 — Template output non-empty for literal text
    - **Property 3: Template output non-empty for literal text**
    - **Validates: Requirements 4.7**
    - Using `rapid`, generate random non-empty literal strings (no `{{` or `}}`), evaluate as template, assert result is non-empty

- [x] 3. Update `GenerateDisplayName` in `internal/domain/peer.go`
  - Change signature to `GenerateDisplayName(prefix string, tmpl string, user *User)`
  - Build `PeerNameTemplateData` from `p` and `user` (user fields default to `""` when `user` is nil)
  - Use effective template: `tmpl` if non-empty, else `"Peer {{.Random}}"`
  - On success set `p.DisplayName` to the result; on error log with `slog.Error` (template string + error) and fall back to `"Peer {ID}"` (current behavior)
  - Add debug log on success: `slog.Debug("peer display name generated from template", "template", tmpl, "displayName", p.DisplayName)`
  - _Requirements: 1.4, 1.5, 2.1, 2.3, 2.4, 6.1_

  - [x] 3.1 Write unit tests for updated `GenerateDisplayName`
    - `TestGenerateDisplayName_WithTemplate`: sets `DisplayName` to template result for a valid template
    - `TestGenerateDisplayName_FallbackOnError`: falls back to legacy behavior when template is invalid
    - `TestGenerateDisplayName_EmptyTemplate`: empty template string triggers legacy behavior
    - Update existing `TestPeer_GenerateDisplayName` to pass the new parameters
    - _Requirements: 1.4, 1.5, 2.3_

  - [x] 3.2 Write property test: Property 4 — Template applied at peer creation
    - **Property 4: Template applied at peer creation**
    - **Validates: Requirements 2.1, 2.5**
    - Using `rapid`, generate random `PeerNameTemplateData` and valid template; call `GenerateDisplayName` and assert `peer.DisplayName == ApplyPeerNameTemplate(tmpl, data)`

- [x] 4. Checkpoint — Ensure all domain-layer tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 5. Wire template into all peer creation paths in `internal/app/wireguard/wireguard_peers.go` and `internal/app/api/v1/backend/provisioning_service.go`
  - [x] 5.1 Update `PreparePeer`: fetch `currentUserObj` from DB using `currentUser.Id`; replace `freshPeer.GenerateDisplayName("")` with `freshPeer.GenerateDisplayName("", m.cfg.Core.PeerTemplateName, currentUserObj)`; add `slog.Debug` log after template is applied (peer identifier + resulting DisplayName)
    - _Requirements: 2.1, 2.5, 3.1_

  - [x] 5.2 Update `CreateDefaultPeer`: fetch `linkedUser` by `userId`; replace `peer.GenerateDisplayName("Default")` with `peer.GenerateDisplayName("Default", m.cfg.Core.PeerTemplateName, linkedUser)`
    - _Requirements: 2.1, 2.5_

  - [x] 5.3 Update `CreateMultiplePeers`: when `m.cfg.Core.PeerTemplateName` is non-empty, skip the `r.Prefix` prepend (template result stands as-is); when template is empty/default, keep existing prefix logic
    - _Requirements: 2.1, 2.5_

  - [x] 5.4 Update provisioning `NewPeer` in `internal/app/api/v1/backend/provisioning_service.go`: fetch `linkedUser` using `req.UserIdentifier`; replace `peer.GenerateDisplayName("API")` with `peer.GenerateDisplayName("API", p.cfg.Core.PeerTemplateName, linkedUser)`
    - _Requirements: 2.1, 2.2, 2.5_

  - [x] 5.5 Write property test: Property 5 — Caller-provided DisplayName is preserved
    - **Property 5: Caller-provided DisplayName is preserved**
    - **Validates: Requirements 2.2, 3.3**
    - Using `rapid`, generate random non-empty display name strings and random template strings; assert that when `req.DisplayName` is non-empty the saved peer's `DisplayName` equals the caller-supplied value

- [x] 6. Add debug logging in `PreparePeer` for template application
  - After `freshPeer.GenerateDisplayName(...)` in `PreparePeer`, add `slog.DebugContext(ctx, "PreparePeer applied peer name template", "peerId", freshPeer.Identifier, "displayName", freshPeer.DisplayName)`
  - _Requirements: 2.1_

- [x] 7. Checkpoint — Ensure all service-layer tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 8. Update documentation in `docs/documentation/configuration/overview.md`
  - Add `peer_template_name` to the default config YAML block under `core:`
  - Add a `### peer_template_name` section under the Core heading describing: default value, env var `WG_PORTAL_CORE_PEER_TEMPLATE_NAME`, purpose, supported variables (`{{.Id}}`, `{{.Random}}`, `{{.Email}}`, `{{.Firstname}}`, `{{.Lastname}}`, `{{.PeerName}}`), capitalization convention, and example values
  - _Requirements: 5.1, 5.2, 5.3, 5.4_

- [x] 9. Final checkpoint — Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for a faster MVP
- The frontend (`PeerEditModal.vue`) requires no changes — it already copies `peers.Prepared.DisplayName` into `formData.DisplayName`, so the template-evaluated name flows through automatically once `PreparePeer` sets it
- Property tests use the [rapid](https://github.com/flyingmutant/rapid) library (already used in the project or add as a test dependency)
- All logging uses `log/slog` following existing codebase patterns
