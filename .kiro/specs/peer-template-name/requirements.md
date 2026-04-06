# Requirements Document

## Introduction

WireGuard Portal currently generates peer display names using a random suffix pattern (e.g., `Peer 9ZU4xXRg`). This feature adds a `peer_template_name` configuration parameter that allows administrators to define a custom naming template for newly created peers. The template supports variable substitution for peer and user attributes (Id, Email, First name, Last name, Peer name). When not configured, the system falls back to the existing naming behavior.

Template variables use Go template field naming conventions (capitalized), e.g., `{{.Email}}` not `{{.email}}`. Example configurations:

- `"Peer {{.Random}}"` — default; keeps current behavior, generates names like `Peer 9ZU4xXRg`
- `"Peer {{.Email}}"` — uses the linked user's email, e.g., `Peer alice@example.com`

## Glossary

- **System**: The WireGuard Portal application.
- **Peer**: A WireGuard peer entry managed by WireGuard Portal, represented by the `Peer` domain struct.
- **DisplayName**: The human-readable name field of a `Peer` used for display and config file naming.
- **peer_template_name**: A new configuration parameter (string) in the `Core` config section that defines a Go template string for generating peer display names.
- **Template_Engine**: The component responsible for evaluating `peer_template_name` template strings and producing a resolved display name.
- **Template_Variable**: A named placeholder within the `peer_template_name` string that is substituted at peer creation time (e.g., `{{.Email}}`, `{{.Random}}`).
- **User**: The `User` domain struct linked to a peer, containing `Identifier`, `Email`, `Firstname`, `Lastname` fields.

## Requirements

### Requirement 1: peer_template_name Configuration Parameter

**User Story:** As an administrator, I want to configure a custom naming template for peers, so that peer display names follow my organization's naming conventions automatically.

#### Acceptance Criteria

1. THE System SHALL provide a `peer_template_name` string field in the `Core` configuration section.
2. WHEN `peer_template_name` is empty or not set, THE System SHALL default to `"Peer {{.Random}}"`, which produces the same output as the existing naming logic (e.g., `Peer 9ZU4xXRg`).
3. THE System SHALL support the following template variables in `peer_template_name`:
   - `{{.Id}}` — the truncated peer identifier
   - `{{.Random}}` — a randomly generated short alphanumeric string (e.g., `9ZU4xXRg`), providing a unique suffix independent of peer identity
   - `{{.Email}}` — the email address of the linked user (empty string if no user is linked)
   - `{{.Firstname}}` — the first name of the linked user (empty string if not set)
   - `{{.Lastname}}` — the last name of the linked user (empty string if not set)
4. WHERE `peer_template_name` is set, THE System SHALL evaluate the template using Go's `text/template` engine.
5. IF `peer_template_name` contains an invalid Go template syntax, THEN THE System SHALL log an error and fall back to the default naming behavior.

### Requirement 2: Apply Template During Peer Creation

**User Story:** As an administrator, I want peer display names to be automatically generated from the template when peers are created, so that I don't have to rename peers manually.

#### Acceptance Criteria

1. WHEN a new peer is created via any creation path (admin UI, self-provisioning, default peer creation, API, bulk creation), THE System SHALL apply `peer_template_name` to generate the peer's `DisplayName` if the template is configured.
2. WHEN a peer is created with an explicit `DisplayName` provided by the caller (e.g., via the REST API `display_name` field), THE System SHALL use the caller-provided name and SHALL NOT apply `peer_template_name`.
3. WHEN `peer_template_name` is configured and the linked user has no `Email`, `Firstname`, or `Lastname`, THE System SHALL substitute those variables with empty strings.
4. WHEN `peer_template_name` is configured and no user is linked to the peer, THE System SHALL substitute all user-related variables (`{{.Email}}`, `{{.Firstname}}`, `{{.Lastname}}`) with empty strings.
5. THE System SHALL apply `peer_template_name` consistently across all peer creation code paths: `GenerateDisplayName`, `CreateDefaultPeer`, `CreateMultiplePeers`, and the provisioning API.

### Requirement 3: Create Peer Dialog UI Pre-population

**User Story:** As an administrator, I want the "Create Peer" dialog to pre-fill the Display Name field with the evaluated template result, so that I can see the generated name before saving and optionally adjust it.

#### Acceptance Criteria

1. WHEN the administrator opens the "Create Peer" dialog in the admin UI, THE System SHALL pre-populate the Display Name field with the result of evaluating the configured `peer_template_name` template (e.g., `"Peer {{.Random}}"` → `Peer 9ZU4xXRg`).
2. WHILE the "Create Peer" dialog is open, THE System SHALL allow the administrator to freely edit the pre-populated Display Name field.
3. WHEN the administrator submits the "Create Peer" form with a manually edited Display Name, THE System SHALL use the administrator-provided value as the peer's `DisplayName` and SHALL NOT re-apply `peer_template_name`.
4. WHEN the administrator submits the "Create Peer" form without modifying the pre-populated Display Name, THE System SHALL use the pre-populated template-evaluated value as the peer's `DisplayName`.
5. IF `peer_template_name` cannot be evaluated at dialog open time, THEN THE System SHALL leave the Display Name field empty, allowing the administrator to enter a name manually.

### Requirement 4: Template Variable Resolution

**User Story:** As an administrator, I want template variables to resolve correctly from peer and user data, so that generated names are accurate and meaningful.

#### Acceptance Criteria

1. WHEN evaluating `peer_template_name`, THE Template_Engine SHALL resolve `{{.Id}}` to the first 8 characters of the peer's public key identifier.
2. WHEN evaluating `peer_template_name`, THE Template_Engine SHALL resolve `{{.Email}}` to the `Email` field of the peer's linked `User`.
3. WHEN evaluating `peer_template_name`, THE Template_Engine SHALL resolve `{{.Firstname}}` to the `Firstname` field of the peer's linked `User`.
4. WHEN evaluating `peer_template_name`, THE Template_Engine SHALL resolve `{{.Lastname}}` to the `Lastname` field of the peer's linked `User`.
5. WHEN evaluating `peer_template_name`, THE Template_Engine SHALL resolve `{{.PeerName}}` to the result of the existing default naming logic (i.e., `Peer {truncated_id}` without any prefix).
6. WHEN evaluating `peer_template_name`, THE Template_Engine SHALL resolve `{{.Random}}` to a randomly generated alphanumeric string of 8 characters (e.g., `9ZU4xXRg`).
7. FOR ALL valid `peer_template_name` templates, THE Template_Engine SHALL produce a non-empty string after variable substitution when at least one variable resolves to a non-empty value or the template contains literal text.

### Requirement 5: Documentation Update

**User Story:** As a developer or administrator, I want the documentation to reflect the new `peer_template_name` configuration parameter, so that I can understand how to configure and use it.

#### Acceptance Criteria

1. THE System documentation SHALL be updated to describe the `peer_template_name` configuration parameter, its default value, and its purpose.
2. THE documentation SHALL list all supported template variables (`{{.Id}}`, `{{.Random}}`, `{{.Email}}`, `{{.Firstname}}`, `{{.Lastname}}`, `{{.PeerName}}`) with a description of each.
3. THE documentation SHALL include example values for `peer_template_name` (e.g., `"Peer {{.Random}}"`, `"Peer {{.Email}}"`).
4. THE documentation SHALL note that template variables use Go template capitalization conventions.

### Requirement 6: Backward Compatibility

**User Story:** As an existing user of WireGuard Portal, I want the system to behave exactly as before when `peer_template_name` is not configured, so that upgrading does not change existing peer naming behavior.

#### Acceptance Criteria

1. WHEN `peer_template_name` is not set in the configuration, THE System SHALL default `peer_template_name` to `"Peer {{.Random}}"`, producing display names identical to the existing `GenerateDisplayName` logic (e.g., `Peer 9ZU4xXRg`).
2. THE System SHALL NOT require `peer_template_name` to be explicitly set; the field SHALL be optional with a default value of `"Peer {{.Random}}"`.
3. WHEN `peer_template_name` is set to an empty string, THE System SHALL treat it as not configured and apply the default value `"Peer {{.Random}}"` to preserve existing naming behavior.

