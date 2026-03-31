For all external authentication providers (LDAP, OIDC, OAuth2), WireGuard Portal can automatically create a local user record upon the user's first successful login.
This behavior is controlled by the `registration_enabled` setting in each authentication provider's configuration.

User information from external authentication sources is merged into the corresponding local WireGuard Portal user record whenever the user logs in.
Additionally, WireGuard Portal supports periodic synchronization of user data from an LDAP directory.

To prevent overwriting local changes, WireGuard Portal allows you to set a per-user flag that disables synchronization of external attributes.
When this flag is set, the user in WireGuard Portal will not be updated automatically during log-ins or LDAP synchronization.

### LDAP Synchronization

WireGuard Portal lets you hook up any LDAP server such as Active Directory or OpenLDAP for both authentication and user sync.
You can even register multiple LDAP servers side-by-side. Details on the log-in process can be found in the [LDAP Authentication](./authentication.md#ldap-authentication) section.

If you enable LDAP synchronization, all users within the LDAP directory will be created automatically in the WireGuard Portal database if they do not exist.
If a user is disabled or deleted in LDAP, the user will be disabled in WireGuard Portal as well.
The synchronization process can be fine-tuned by multiple parameters, which are described below.

#### Synchronization Parameters

To enable the LDAP sycnhronization this feature, set the `sync_interval` property in the LDAP provider configuration to a value greater than "0".
The value is a string representing a duration, such as "15m" for 15 minutes or "1h" for 1 hour (check the [exact format definition](https://pkg.go.dev/time#ParseDuration) for details).
The synchronization process will run in the background and synchronize users from LDAP to the database at the specified interval.
Also make sure that the `sync_filter` property is a well-formed LDAP filter, or synchronization will fail.

##### Limiting Synchronization to Specific Users

Use the `sync_filter` property in your LDAP provider block to restrict which users get synchronized.
It accepts any valid LDAP search filter, only entries matching that filter will be pulled into the portal's database.

For example, to import only users with a `mail` attribute:
```yaml
auth:
  ldap:
    - id: ldap
      # ... other settings
      sync_filter: (mail=*)
```

##### Disable Missing Users

If you set the `disable_missing` property to `true`, any user that is not found in LDAP during synchronization will be disabled in WireGuard Portal.
All peers associated with that user will also be disabled.

If you want a user and its peers to be automatically re-enabled once they are found in LDAP again, set the `auto_re_enable` property to `true`.
This will only re-enable the user if they were disabled by the synchronization process. Manually disabled users will not be re-enabled.

##### Interface-specific Access Materialization

If `interface_filter` is configured in the LDAP provider, the synchronization process will evaluate these filters for each enabled user.
The results are materialized in the `interfaces` table of the database in a hidden field.
This materialized list is used by the backend to quickly determine if a user has permission to provision peers for a specific interface, without having to query the LDAP server for every request.
The list is refreshed every time the LDAP synchronization runs.
For more details on how to configure these filters, see the [Authentication](./authentication.md#interface-specific-provisioning-filters) section.

---

### SCIM Provisioning

WireGuard Portal supports [SCIM v2.0](https://datatracker.ietf.org/doc/html/rfc7644) (System for Cross-domain Identity Management) for automated user provisioning and deprovisioning from external identity providers such as Microsoft Entra ID (Azure AD), Okta, or OneLogin.

Unlike LDAP synchronization which pulls users on a schedule, SCIM is push-based — the identity provider sends real-time updates to WireGuard Portal whenever users are created, updated, or removed.

#### Enabling SCIM

Add the following to your configuration file:

```yaml
scim:
  enabled: true
  bearer_token: "your-secret-scim-token-here"
  delete_action: "disable"
  provider_name: "MyOIDCProvider"  # must match your OIDC provider_name
```

The `provider_name` should match the `provider_name` of your OIDC provider in the `auth.oidc` section.
This ensures that users provisioned via SCIM share the same authentication source as users who log in via OIDC, preventing duplicate entries.

For all available options, see the [SCIM configuration reference](../configuration/overview.md#scim).

#### Setting Up Your Identity Provider

Configure your identity provider's SCIM integration with the following settings:

- **Tenant URL:** `https://your-wg-portal-url/scim/v2`
- **Authentication:** Bearer token (use the value from `bearer_token` in your config)

#### Supported Operations

| Operation           | Description                                                                                          |
| ------------------- | ---------------------------------------------------------------------------------------------------- |
| Create user         | Creates a new user in WireGuard Portal, or updates an existing user if the identifier already exists |
| Update user (PUT)   | Replaces user profile attributes                                                                     |
| Update user (PATCH) | Partially updates user attributes (e.g., disable via `active: false`)                                |
| Delete user         | Disables or deletes the user, depending on the `delete_action` setting                               |
| List users          | Returns all users (supports filtering and pagination)                                                |
| Get user            | Returns a single user by ID                                                                          |

#### Attribute Mapping

| SCIM Attribute          | WireGuard Portal Field | Notes                                    |
| ----------------------- | ---------------------- | ---------------------------------------- |
| `userName`              | User Identifier        | Required, unique                         |
| `externalId`            | External ID            | Set by the identity provider             |
| `name.givenName`        | Firstname              |                                          |
| `name.familyName`       | Lastname               |                                          |
| `emails[primary].value` | Email                  | Uses the primary email, or the first one |
| `phoneNumbers[0].value` | Phone                  |                                          |
| `active`                | Disabled (inverted)    | `false` disables the user                |

#### Interaction with OIDC Login

When `provider_name` is configured, SCIM and OIDC work together seamlessly:

- If a user logs in via OIDC first, then is synced via SCIM, the SCIM sync updates their profile without creating a duplicate authentication source.
- If a user is provisioned via SCIM first, they can log in via the linked OIDC provider immediately.
- Disabling a user via SCIM (`active: false`) prevents them from logging in via any method.

#### Azure Entra ID Notes

WireGuard Portal includes compatibility for Azure Entra ID's SCIM client, which sends boolean values as strings (e.g., `"False"` instead of `false`). This is handled automatically.

##### Configuring `externalId` Mapping

By default, Azure Entra ID maps `mailNickname` to the SCIM `externalId` attribute. This is not ideal because `mailNickname` is not a stable unique identifier — it can change and may not be globally unique.

It is recommended to change this mapping to `objectId` in the Entra ID provisioning configuration:

1. Open your Enterprise Application in the Azure portal
2. Go to **Manage** - **Provisioning** → **Attribute mapping** → **Provision Microsoft Entra ID Users**
3. Find the `externalId` target attribute and edit the mapping:
    - Change **Source attribute** from `mailNickname` to `objectId`
4. Save the mapping

The `objectId` is immutable and globally unique in Azure AD, which allows WireGuard Portal to reliably correlate users even if their UPN or email changes.

The `externalId` attribute is part of the SCIM core schema and is automatically included in the `/Schemas` and `/ResourceTypes` endpoint responses — no additional schema configuration is needed on the WireGuard Portal side.
