# Этап 3. Runtime config platform

## Цель этапа

Отделить bootstrap-конфигурацию от рабочей runtime-конфигурации и сделать БД единственным источником active runtime state.

## Почему этот этап отдельный

Это самый чувствительный startup concern во всей программе работ. Его нельзя смешивать ни с authz migration, ни с SCIM.

## Scope MR

- новая DB model конфигурации
- bootstrap/runtime split
- `--reconfigure`
- validate/apply/history API foundation

## Что реализовать

### 1. Новая config model в БД

Минимум:

- `active`
- `draft`
- `history`
- validation metadata

### 2. Новый startup flow

Обычный runtime должен:

1. открыть БД
2. выполнить миграции
3. загрузить active config из БД
4. провалидировать active config
5. только потом запускать app runtime

### 3. `--reconfigure`

Отдельный CLI flow:

- читает file config
- валидирует
- сохраняет как DB revision
- не запускает runtime

### 4. Backend API foundation

Нужно подготовить surfaces:

- read active
- read draft
- save draft
- validate
- apply
- read history

## Что не входит в этап

- полный admin UI
- SCIM business logic
- unified lifecycle

## План внедрения

1. Спроектировать DB-backed config representation и определить состав bootstrap config, который останется file/env based.
2. Добавить сущности конфигурации и реализовать миграцию `M2`.
3. Разделить startup path на bootstrap phase и runtime phase.
4. Реализовать загрузку active config из БД и блокировку обычного старта без валидной active revision.
5. Реализовать CLI режим `--reconfigure` как controlled file-to-DB import flow.
6. Подготовить backend API для draft/read/validate/apply/history.
7. Прогнать сценарии empty DB, existing DB и upgrade существующей инсталляции.

## Миграция

`M2`

## Критерий готовности

- обычный runtime больше не зависит от file config
- `--reconfigure` работает как controlled import path
- active/draft/history существуют в БД
- upgrade path покрыт тестами
