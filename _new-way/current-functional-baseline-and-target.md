# Функциональный baseline и целевое состояние

## Назначение

Этот документ фиксирует:

- текущий функционал, подтвержденный кодом репозитория
- ожидаемый функционал после выполнения ТЗ из папки `_new-way`

Документ опирается на фактическую реализацию backend, frontend, API, config и database layers.

## 1. Текущий функционал

## 1.1 Общая платформа

Система уже является рабочим WireGuard Portal с:

- Go backend
- Vue frontend
- SQL database
- REST API
- audit log
- statistics и Prometheus metrics
- self-service и admin scenarios

Поддерживаемые базы:

- SQLite
- MySQL
- MS SQL
- Postgres

## 1.2 Управление WireGuard-интерфейсами

Администратор может:

- просматривать список интерфейсов
- создавать интерфейс
- редактировать интерфейс
- удалять интерфейс
- импортировать существующие интерфейсы
- просматривать связанный список peer-ов
- получать конфиг интерфейса
- сохранять конфиг интерфейса в файл при включенном `SaveConfig`
- применять peer defaults к peer-ам интерфейса

У интерфейсов уже есть:

- identifier
- display name
- тип `server|client|any`
- backend
- ключи
- listen port
- addresses
- DNS / DNS search
- MTU
- firewall mark
- routing table
- hooks pre/post up/down
- peer defaults

## 1.3 Управление peer-ами

Система уже поддерживает:

- просмотр peer-ов интерфейса
- создание одного peer-а
- массовое создание peer-ов
- редактирование peer-а
- удаление peer-а
- массовые операции
- просмотр connection status и трафика
- выдачу peer config
- QR code
- отправку конфигурации по email

## 1.4 Пользователи

Система уже поддерживает:

- просмотр списка пользователей
- создание пользователя
- редактирование пользователя
- удаление пользователя
- bulk delete / enable / disable / lock / unlock
- просмотр peer-ов пользователя
- просмотр stats пользователя
- просмотр интерфейсов пользователя
- включение и отключение API token
- смену пароля

У пользователя уже есть:

- identifier
- email
- имя / фамилия
- телефон
- отдел
- notes
- disable / lock состояния
- API token
- WebAuthn credentials
- несколько authentication sources

## 1.5 Аутентификация

В коде уже реализованы:

- локальная аутентификация по БД
- LDAP authentication
- OAuth authentication
- OIDC authentication
- WebAuthn login и registration
- session-based web auth
- Basic Auth + API token для API

Также уже есть:

- список внешних login providers
- OAuth/OIDC callback flow
- default admin user при старте

## 1.6 Авторизация

Текущая модель авторизации:

- бинарная
- основана на `IsAdmin`

Это означает:

- `admin` может управлять системой
- обычный пользователь ограничен собственными данными и self-service сценариями

Отдельной role model сейчас нет.

## 1.7 Ограничение доступа к интерфейсам

Сейчас есть только частичная реализация:

- LDAP-based restriction через `InterfaceFilter`
- materialized `LdapAllowedUsers`

Единой group-based access model для всех identity-источников нет.

## 1.8 Аудит и наблюдаемость

Уже реализовано:

- audit log
- audit events для auth/interface/peer
- frontend audit screen
- statistics collection
- Prometheus metrics
- WebSocket/live updates для части статусов

При этом пока нет:

- отдельного SCIM audit source
- provider status registry
- preflight endpoint
- structured admin diagnostics model
- rate limiting platform для auth/scim

## 1.9 Конфигурация приложения

Сейчас конфигурация:

- читается из файла и env
- валидируется и санитизируется на startup path
- используется как runtime source напрямую

Также уже есть:

- автоматические миграции БД
- `ConfigStoragePath` для сохранения WireGuard config files

Но пока нет:

- active config в БД
- draft/history/apply model
- `--reconfigure` как controlled import flow
- configuration CRUD API
- admin UI для настройки всей системы

## 1.10 Frontend

Во frontend уже доступны:

- home
- login
- interfaces
- users
- profile
- settings
- audit
- key generator
- IP calculator

Фактически frontend уже покрывает:

- admin workflows по пользователям, интерфейсам и peer-ам
- self-service profile workflows
- passkeys / WebAuthn settings
- audit screen

Но пока не покрывает:

- системную runtime-конфигурацию приложения
- SCIM settings
- provider diagnostics/status/preflight
- role/access mapping administration

## 1.11 Что в коде отсутствует

Подтверждено как отсутствующий функционал:

- SCIM subsystem
- SCIM Users API
- SCIM Groups API
- SCIM token management
- SCIM deprovision policies
- role model `admin/user/monitoring`
- policy layer
- unified normalization layer
- unified lifecycle orchestration
- DB-backed runtime config platform
- admin configuration UI

## 2. Ожидаемый функционал после выполнения ТЗ

## 2.1 Общая платформа

Система должна остаться рабочим WG Portal, но дополнительно превратиться в enterprise-oriented identity-aware platform для управления доступом и provisioning.

Новый функционал должен быть надстроен над текущими подсистемами, а не заменять их целиком.

## 2.2 Аутентификация и безопасность

После выполнения ТЗ ожидается:

- безопасное отображение login providers без HTML injection
- строгая validation `external_url` и callback configuration
- OIDC как preferred path, plain OAuth как fallback
- разделение user-facing и admin-facing auth errors
- auth-related rate limiting

## 2.3 Новая модель авторизации

Вместо бинарного `IsAdmin` ожидается:

- роли `admin`, `user`, `monitoring`
- policy layer для backend permission checks
- role-aware frontend behavior
- group-based role assignment
- group-based interface access assignment

## 2.4 Runtime-конфигурация

После выполнения ТЗ система должна поддерживать:

- хранение runtime-конфигурации в БД
- `active` config
- `draft` config
- `history`
- validate/apply flow
- startup runtime только из БД
- `--reconfigure` как единственный file-to-DB import path

## 2.5 Admin configuration UI

Через Web UI должно стать возможно:

- читать active config
- редактировать draft config
- валидировать изменения
- применять конфигурацию
- просматривать history
- управлять auth provider settings
- управлять mappings и access policies
- просматривать diagnostics/status/preflight

## 2.6 Observability и diagnostics

После выполнения ТЗ ожидается:

- auth provider status registry
- admin preflight endpoint
- structured diagnostics responses
- enrichened audit model
- surfaces для admin/monitoring ролей

## 2.7 SCIM Users

Должно появиться:

- SCIM authentication path
- SCIM Users API
- create/read/update/patch/delete flows
- SCIM-compatible error model
- immutable `userName`
- mapping SCIM user representation на текущую user model
- audit integration для SCIM operations

## 2.8 SCIM Groups

На целевом состоянии также должно появиться:

- SCIM Groups API
- persistent groups
- memberships
- group-to-role mappings
- group-to-interface-access mappings

## 2.9 Identity normalization и lifecycle

После выполнения ТЗ ожидается:

- единый `ExternalIdentity` contract
- normalization pipeline для OAuth/OIDC/LDAP/SCIM
- email normalization
- trust-level policy
- source precedence policy
- lifecycle orchestration для multi-source identity updates

## 2.10 Enterprise SCIM behavior

SCIM subsystem в целевом состоянии должен дополнительно поддерживать:

- deprovision policy
- DB-side filtering
- token rotation / revoke / expiration
- resolved mapping diagnostics
- документацию capability matrix и ограничений

## 2.11 Что должно сохраниться

При выполнении ТЗ обязательно нужно сохранить уже существующий функционал:

- управление интерфейсами
- управление peer-ами
- управление пользователями
- self-service profile
- WebAuthn
- audit
- statistics
- API v0/v1
- existing backend integrations

## 3. Короткое сравнение: сейчас и после ТЗ

| Область | Сейчас | После ТЗ |
| --- | --- | --- |
| Auth | DB/LDAP/OAuth/OIDC/WebAuthn | то же + hardened auth flow и diagnostics |
| Authz | `IsAdmin` | roles + policy layer |
| Interface access | частично LDAP-only | общая access model |
| Config | file/env runtime source | DB-backed runtime config |
| Config UI | отсутствует | полный admin config UI |
| Diagnostics | ограничены логами и audit | status/preflight/structured diagnostics |
| SCIM | отсутствует | SCIM Users + SCIM Groups |
| Identity merge | разрозненный | unified normalization/lifecycle |
| Audit | auth/interface/peer | расширенный, включая SCIM и config operations |

