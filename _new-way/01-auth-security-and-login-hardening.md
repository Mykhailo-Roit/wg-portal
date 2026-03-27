# Этап 1. Auth security и login hardening

## Цель этапа

Первый этап должен улучшить безопасность и предсказуемость уже существующего auth/login path без изменения модели данных.

Этап намеренно локальный: его удобно ревьюить и мерджить отдельно, потому что он опирается только на уже существующие модули `config`, `auth`, `api`, `frontend`.

## Почему этот этап первый

В текущем коде уже есть:

- OIDC/OAuth/LDAP/WebAuthn
- login screen
- callback flows
- startup config loading

При этом уже видны конкретные локальные проблемы:

- `display_name` провайдера рендерится небезопасно
- `external_url` и callback path валидируются недостаточно явно
- user/admin ошибки SSO смешаны

Это можно исправить до любых миграций и до внедрения SCIM.

## Scope MR

- backend validation вокруг существующего auth startup path
- frontend login rendering
- auth error classification
- документация и config examples по существующим auth-провайдерам

## Что реализовать

### 1. Безопасное отображение login providers

- убрать HTML rendering для имени провайдера
- считать `display_name` plain text значением
- санитизировать значение на config loading path

### 2. Startup validation для `external_url`

Нужно ввести явный validator, который проверяет:

- URL парсится
- схема допустима
- локальные dev URL допустимы
- публичный `http` помечается как ошибка или warning по policy
- callback URL для OAuth/OIDC/WebAuthn строится предсказуемо

### 3. Явная политика OIDC vs plain OAuth

На базе уже существующих OIDC и OAuth provider-ов:

- OIDC считается предпочтительным вариантом
- plain OAuth считается fallback
- startup должен уметь логировать warning для рискованных конфигураций

### 4. Разделение user-facing и admin-facing auth ошибок

- безопасные сообщения пользователю
- подробные причины в логах и diagnostics model следующего этапа
- единые machine-readable коды ошибок

## Что не входит в этап

- новые роли
- runtime config в БД
- SCIM
- новая БД-модель

## План внедрения

1. Зафиксировать текущие auth/login сценарии тестами и собрать список небезопасных точек в существующем flow.
2. Убрать HTML rendering имени провайдера во frontend и привести `display_name` к plain text semantics.
3. Добавить backend sanitize/validation helper для provider display name на config loading path.
4. Ввести startup validator для `external_url`, callback URL и связанных auth settings.
5. Добавить policy и structured warnings для сценариев `OIDC preferred / OAuth fallback`.
6. Вынести auth error classification в отдельный слой и разделить user-facing и admin-facing ответы.
7. Обновить config examples, документацию и интеграционные тесты на login/callback path.

## Миграция

Не требуется.

## Критерий готовности

- login page больше не имеет HTML injection path для provider name
- startup validation по `external_url` и callback config детерминирована
- auth errors разделены на user-facing и admin-facing уровни
- изменения покрыты unit/integration тестами по существующему auth path
