# Этап 5. SCIM Users foundation

## Цель этапа

Реализовать первую рабочую SCIM-подсистему поверх уже существующих:

- user model
- authz model
- runtime config platform
- diagnostics foundation

## Почему SCIM только здесь

В текущем коде SCIM отсутствует полностью. Чтобы не получить слишком большой MR, сначала должны быть готовы:

- новая authz model
- DB-backed config platform
- diagnostics/rate limiting foundation

После этого SCIM можно вводить как отдельную и обозримую новую подсистему.

## Scope MR

- SCIM authentication
- SCIM Users handlers/service layer
- mapping к текущей user model
- SCIM-specific audit integration
- config integration через DB-backed config model

## Что реализовать

### 1. SCIM authentication

- отдельный service-to-service auth path
- integration с новой config model
- preparation for future multi-token support

### 2. SCIM Users API

Минимальный обязательный объем:

- `GET /Users`
- `GET /Users/{id}`
- `POST /Users`
- `PATCH /Users/{id}`
- `PUT /Users/{id}`
- `DELETE /Users/{id}` или эквивалентный совместимый деактивационный path

### 3. Mapping на текущую user model

Нужно аккуратно связать SCIM user payload с уже существующим `domain.User`, не ломая текущие auth/users paths.

### 4. Immutable field rules

На этом этапе сразу зафиксировать:

- `userName` immutable
- SCIM-compatible error responses

### 5. Audit и diagnostics integration

- SCIM должен появиться как отдельный источник событий
- ошибки и ответы должны вписываться в diagnostics contract

## Что не входит в этап

- SCIM Groups
- unified lifecycle orchestration
- advanced merge/precedence policy
- enterprise-grade token rotation

## План внедрения

1. Определить минимальный SCIM subsystem boundary поверх уже существующих `users`, `authz`, `config` и `audit`.
2. Реализовать SCIM authentication path и привязать его к DB-backed config model.
3. Добавить SCIM Users service layer и handlers для минимального обязательного набора endpoints.
4. Спроектировать mapping между SCIM user representation и текущей user model без разрушения существующих flows.
5. Встроить immutable field rules и SCIM-compatible error handling.
6. Подключить SCIM к audit и diagnostics infrastructure.
7. Покрыть create/read/update/patch/delete сценарии тестами и проверить контрактную совместимость foundation.

## Миграция

Отдельная migration не обязательна, если SCIM foundation опирается на уже введенные модели.

## Критерий готовности

- в проекте есть рабочий SCIM Users surface
- SCIM интегрирован в auth/config/audit framework
- базовые операции покрыты тестами
- подсистема готова к следующему архитектурному этапу, а не к переписыванию
