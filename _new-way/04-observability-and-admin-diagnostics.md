# Этап 4. Observability и admin diagnostics

## Цель этапа

Сделать существующие auth/config subsystems операционно прозрачными до появления SCIM и lifecycle orchestration.

## Почему этот этап здесь

После стабилизации authz и runtime-config foundation можно безопасно вводить наблюдаемость и админские diagnostics contracts, не смешивая их с первичной реализацией SCIM.

## Scope MR

- provider status registry
- auth/config preflight checks
- rate limiting
- audit enrichment
- diagnostics contracts

## Что реализовать

### 1. Auth provider status model

Для уже существующих OIDC/OAuth/LDAP provider-ов:

- init status
- availability status
- last error
- effective callback URL

### 2. Preflight endpoint

Admin-only API для проверки:

- auth provider readiness
- callback consistency
- config validity для связанных подсистем

### 3. Rate limiting

Защитить:

- plain login
- SSO init
- SSO callback
- будущие SCIM endpoints

На этом этапе достаточно ввести общую rate limiting platform на auth-related endpoints.

### 4. Diagnostics contract

Нужен единый формат:

- code
- severity
- message
- recommendation
- admin detail

### 5. Audit enrichment

Расширить существующую audit model так, чтобы она была готова к будущему SCIM source и config operations.

## Что не входит в этап

- сама реализация SCIM
- UI для полной настройки системы
- lifecycle merge logic

## План внедрения

1. Ввести единый diagnostics contract и встроить его в уже существующие auth/config error paths.
2. Реализовать provider status registry для текущих OIDC/OAuth/LDAP integrations.
3. Добавить admin-only preflight endpoint для проверки auth/config readiness.
4. Встроить rate limiting platform в auth-related endpoints с конфигурируемыми policy settings.
5. Расширить audit model и recorder так, чтобы они были готовы к новым operational events.
6. Добавить API tests и integration tests для status/preflight/rate limiting behavior.
7. Подготовить backend contracts, на которые затем сможет опереться admin UI.

## Миграция

По возможности без отдельной schema migration.

## Критерий готовности

- у администратора есть status/preflight surfaces
- auth endpoints защищены rate limiting
- diagnostics responses стандартизованы
- audit model готова к новым subsystem events
