# Этап 7. SCIM Groups и доведение до целевого состояния

## Цель этапа

Финальный этап собирает вместе:

- persistent groups
- SCIM Groups
- enterprise behaviors вокруг SCIM
- финальную lifecycle/source precedence model

## Почему это отдельный финальный этап

Это самый интеграционный этап программы. Его нельзя смешивать с первичной реализацией SCIM Users или базовым lifecycle layer.

## Scope MR

- group storage
- memberships
- SCIM Groups handlers
- enterprise SCIM behaviors
- финальная связка frontend/backend diagnostics и config

## Что реализовать

### 1. Persistent group model

Нужно ввести:

- groups
- memberships
- group-to-role mappings
- group-to-interface-access mappings

### 2. SCIM Groups API

Минимальный объем:

- чтение групп
- membership updates
- влияние групп на роли
- влияние групп на доступ к интерфейсам

### 3. Enterprise completion для SCIM

Нужно довести подсистему до целевого состояния:

- deprovision policy
- DB-side filtering для list/search
- multi-token / rotation / revoke
- resolved mapping diagnostics
- итоговая документация

### 4. Завершение lifecycle model

- persistent source attribution
- финальная precedence policy
- корректный пересчет access и ролей при изменении групп

## Что не входит в этап

- отдельные новые большие платформенные рефакторы вне already-built foundations

## План внедрения

1. Добавить persistent group model и реализовать миграцию `M3`.
2. Реализовать membership materialization и связать группы с role/interface access mappings.
3. Добавить SCIM Groups handlers и встроить их в уже существующий SCIM foundation.
4. Расширить SCIM subsystem enterprise-возможностями: deprovision policy, filtering, token rotation/revoke.
5. Завершить persistent source attribution и final precedence behavior в lifecycle layer.
6. Довести diagnostics, audit и документацию до итогового состояния.
7. Прогнать end-to-end тесты на multi-source сценариях и upgrade path до целевой архитектуры.

## Миграция

`M3`

## Критерий готовности

- SCIM Users и Groups полностью работают в рамках новой платформы
- роли и interface access корректно пересчитываются
- enterprise SCIM behaviors доступны и покрыты тестами
- итоговая архитектура соответствует целевой программе работ
