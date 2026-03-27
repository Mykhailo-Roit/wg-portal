# Этап 2. Authorization и interface access

## Цель этапа

Перевести проект от бинарной проверки `IsAdmin` к расширяемой authorization model, не трогая пока runtime config и SCIM.

## Почему этот этап отдельный

В текущем коде `IsAdmin` используется во многих местах backend и frontend. Это широкий рефактор permission path и его нельзя смешивать с runtime config или SCIM.

Это отдельный MR block, потому что:

- меняется много authorization checks
- нужна миграция БД
- нужен аккуратный compatibility bridge

## Scope MR

- domain/authz model
- DB migration
- backend policy layer
- минимальные frontend изменения для новых ролей
- generalization существующего interface access поведения

## Что реализовать

### 1. Ввести новую role model

Минимальные роли:

- `admin`
- `user`
- `monitoring`

### 2. Ввести policy layer

Нужны централизованные проверки доступа вместо scattered `IsAdmin`:

- `CanManageUsers`
- `CanManageInterfaces`
- `CanManagePeers`
- `CanReadAudit`
- `CanReadDiagnostics`
- `CanManageConfiguration`

### 3. Обобщить доступ к интерфейсам

Текущее LDAP-only ограничение доступа к интерфейсам нужно превратить в общую access model:

- persistent assignments
- подготовка к будущим group mappings
- compatibility с существующим поведением

### 4. Сохранить upgrade compatibility

Обязательный backfill:

- `IsAdmin=true -> admin`
- `IsAdmin=false -> user`

`IsAdmin` временно сохраняется как compatibility field до полной вычистки старых веток.

## Что не входит в этап

- runtime config в БД
- SCIM
- unified lifecycle
- admin config UI

## План внедрения

1. Описать новую authz model поверх текущей `domain.User` и определить минимальный role set.
2. Добавить новые сущности БД и реализовать миграцию `M1` с безопасным backfill из `IsAdmin`.
3. Ввести policy layer с совместимым adapter-слоем для старых проверок.
4. Перевести backend permission checks в критических сервисах на policy API.
5. Обобщить существующий интерфейсный доступ в отдельную access model вместо LDAP-only ad-hoc логики.
6. Обновить API/session/frontend surfaces, которые сейчас жестко завязаны на `IsAdmin`.
7. Прогнать upgrade-тесты на существующей БД и проверить сценарии `admin`, `user`, `monitoring`.

## Миграция

`M1`

## Критерий готовности

- роли реально управляют доступом
- основные backend checks используют policy layer
- существующие установки обновляются без потери доступа
- интерфейсный доступ больше не завязан только на LDAP ad-hoc логику
