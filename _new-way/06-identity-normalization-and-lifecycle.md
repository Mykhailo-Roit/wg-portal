# Этап 6. Identity normalization и lifecycle

## Цель этапа

После появления SCIM Users нужно убрать расхождения между существующими внешними identity-потоками и новой SCIM-подсистемой.

## Почему это не раньше

В текущем коде нет общего normalization layer. Но строить его до появления SCIM foundation рискованно: легко переусложнить модель без реального интеграционного потребителя.

После этапа 5 появляются все нужные источники:

- OAuth
- OIDC
- LDAP
- SCIM

## Scope MR

- `ExternalIdentity` contract
- adapters и normalization pipeline
- lifecycle orchestration service
- source precedence rules

## Что реализовать

### 1. Unified normalized identity contract

Минимально:

- identifier
- email
- firstname
- lastname
- phone
- department
- groups
- roles
- source metadata

### 2. Общий normalization pipeline

Через него должны проходить:

- OIDC claims
- OAuth user info
- LDAP attributes
- SCIM payload

### 3. Email normalization

- trim
- lowercase
- validation
- verified / not verified states, где доступны

### 4. Lifecycle orchestration

Единая логика для:

- create
- update
- disable
- merge
- source attribution

### 5. Source precedence

На этом этапе нужно централизовать, какой источник влияет на effective value поля.

## Что не входит в этап

- SCIM Groups API
- финальная persistent group model
- финальные enterprise extras SCIM

## План внедрения

1. Определить `ExternalIdentity` contract на основе полей, уже используемых в OAuth/OIDC/LDAP/SCIM paths.
2. Выделить adapters для существующих внешних источников и свести их к единому normalization pipeline.
3. Реализовать email normalization и source metadata handling как отдельные reusable компоненты.
4. Вынести merge/update decisions в lifecycle orchestration service.
5. Добавить вычислительный source precedence resolver для effective values.
6. Перевести существующие external identity flows на новый pipeline по одному источнику за раз.
7. Подтвердить一致ность поведения через unit/integration tests на multi-source сценариях.

## Миграция

Возможна additive подготовка, но основная group/lifecycle storage migration переносится на следующий этап.

## Критерий готовности

- внешние identity-источники используют совместимый normalization path
- update/merge логика перестает быть scattered
- lifecycle rules централизованы
