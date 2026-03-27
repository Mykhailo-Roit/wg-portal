# ТЗ: дорожная карта развития WG Portal от текущего кода к SCIM, новой authz и DB-backed config

## Назначение

Этот комплект ТЗ пересобран заново и опирается только на текущую кодовую базу репозитория.

Из документа исключены предположения из старых вариантов ТЗ. Все этапы построены от реально существующих подсистем:

- `auth`
- `users`
- `wireguard`
- `audit`
- `config`
- `api v0/v1`
- `frontend`
- `database migrations`

## Что есть в коде сейчас

Подтверждено по проекту:

- есть полноценный backend и frontend для управления интерфейсами, peer-ами и пользователями
- поддерживаются DB auth, LDAP, OAuth, OIDC и WebAuthn
- API и web-сессии уже существуют
- есть audit log, statistics и Prometheus metrics
- есть автоматические миграции БД
- есть частичная LDAP-логика для ограничения доступа к интерфейсам
- есть централизованный config loader, но он file/env based

Подтверждено как ограничение текущего кода:

- нет SCIM
- нет role model кроме `IsAdmin`
- нет policy layer
- нет DB-backed runtime configuration
- нет config CRUD/validate/apply API
- нет admin UI для системной конфигурации
- нет unified identity normalization/lifecycle layer

## Окончательная цель

На базе текущего кода нужно прийти к состоянию, в котором:

- SCIM реализован как новая подсистема поверх существующей user/auth/api модели
- авторизация переведена с `IsAdmin` на роли и policy checks
- доступ к интерфейсам определяется не ad-hoc, а общей access model
- runtime-конфигурация хранится в БД и управляется через controlled flow
- администратор настраивает систему через Web UI
- внешние identity-источники и SCIM сходятся в совместимый lifecycle

## Принципы разбиения этапов

Этапы оптимизированы под удобные GitHub merge requests:

- один этап = один bounded context или одна четкая интеграционная связка
- migration-sensitive изменения не смешиваются с крупным frontend
- authz, runtime config и SCIM разделены на отдельные серии MR
- каждый этап дает законченное промежуточное состояние системы
- этапы основаны только на реально существующих кодовых опорах

## Новый план этапов

| Этап | Файл | Основной фокус | Причина отдельного MR |
| --- | --- | --- | --- |
| 1 | `01-auth-security-and-login-hardening.md` | hardening существующего auth/login/config startup path | локальные изменения без миграций |
| 2 | `02-authorization-and-interface-access.md` | переход с `IsAdmin` к ролям и access policy | широкий backend refactor и первая миграция |
| 3 | `03-runtime-config-platform.md` | runtime config в БД и `--reconfigure` | отдельный рискованный startup concern |
| 4 | `04-observability-and-admin-diagnostics.md` | status/preflight/audit/rate limiting/config diagnostics | отдельный operational layer |
| 5 | `05-scim-users-foundation.md` | первичная реализация SCIM Users | новая подсистема поверх уже стабилизированных опор |
| 6 | `06-identity-normalization-and-lifecycle.md` | общий normalization и lifecycle service | архитектурная консолидация после SCIM foundation |
| 7 | `07-scim-groups-and-enterprise-completion.md` | SCIM Groups и доведение до целевого состояния | финальная интеграционная сборка |

## План миграций

- `M1`: новая authorization model
- `M2`: runtime configuration в БД
- `M3`: groups и lifecycle storage

## ASCII диаграмма новой последовательности

```text
Current WG Portal
|
+-- Existing stable subsystems
|   +-- Users / Peers / Interfaces
|   +-- Auth: DB / LDAP / OAuth / OIDC / WebAuthn
|   +-- Audit / Metrics / API / Frontend
|   +-- File/env config loader
|
+-- Gaps
    +-- No SCIM
    +-- No roles/policy layer
    +-- No DB-backed runtime config
    +-- No unified identity lifecycle

Implementation path optimized for GitHub MR
|
+-- [01] Auth Security + Login Hardening
|   +-- safe provider rendering
|   +-- external_url / callback validation
|   +-- cleaner SSO failure model
|
+-- [02] Authorization + Interface Access      [M1]
|   +-- roles
|   +-- policy checks
|   +-- group-based access model
|
+-- [03] Runtime Config Platform               [M2]
|   +-- active config in DB
|   +-- draft/history/apply
|   +-- --reconfigure
|
+-- [04] Observability + Admin Diagnostics
|   +-- provider status
|   +-- preflight
|   +-- rate limiting
|   +-- richer audit/diagnostics
|
+-- [05] SCIM Users Foundation
|   +-- SCIM auth
|   +-- Users API
|   +-- mapping to current user model
|
+-- [06] Identity Normalization + Lifecycle
|   +-- ExternalIdentity
|   +-- shared normalization
|   +-- lifecycle orchestration
|
+-- [07] SCIM Groups + Enterprise Completion   [M3]
    +-- groups and memberships
    +-- SCIM Groups API
    +-- final source precedence and enterprise behaviors
```

## Итоговые критерии приемки

- SCIM Users и затем SCIM Groups реализованы
- роли `admin`, `user`, `monitoring` работают через policy layer
- runtime config хранится в БД
- file config применяется только через `--reconfigure`
- admin UI покрывает обязательные runtime settings
- внешние identity-источники и SCIM используют совместимый lifecycle
- upgrade существующих установок выполняется автоматически и предсказуемо
