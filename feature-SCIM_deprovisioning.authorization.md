# Детальный анализ: SSO и SCIM в `wg-portal`

## Контекст

Документ собран по коду ветки `feature/SCIM_deprovisioning` и посвящён двум функциональным областям:

- подключение и использование авторизации через SSO-провайдеров
- функционал SCIM

Анализ основан на реализации в:

- `internal/app/auth/...`
- `internal/app/api/v0/handlers/...`
- `internal/app/api/scim/...`
- `internal/config/...`
- `frontend/src/views/LoginView.vue`
- `docs/documentation/...`

## 1. Функционал подключения авторизации через SSO-провайдеров

## 1.1. Что именно считается SSO в этом проекте

В проекте под SSO-функционалом фактически реализованы:

- `OIDC` провайдеры
- `OAuth2` провайдеры
- частично связанный enterprise login через `LDAP`, но это не browser-based SSO в классическом смысле

Если говорить именно про внешних SSO-провайдеров для web login flow, ключевой функционал сосредоточен в:

- `auth.oidc`
- `auth.oauth`
- UI-кнопках внешнего логина
- backend flow `init -> redirect -> callback -> user sync -> session`

## 1.2. Поддерживаемые типы провайдеров

### OpenID Connect

Поддерживаются OIDC-провайдеры с authorization code flow.

Типовые примеры из документации:

- Google
- Entra ID / Azure AD
- Keycloak

Код использует библиотеку `go-oidc`, а значит:

- выполняется discovery через `base_url`
- используется верификация `id_token`
- используется проверка `nonce`

### Plain OAuth2

Поддерживаются и обычные OAuth2-провайдеры без OIDC discovery.

Для такого провайдера нужно явно задать:

- `auth_url`
- `token_url`
- `user_info_url`
- `scopes`

В этом режиме портал сам получает access token и затем отдельно вызывает user info endpoint.

## 1.3. Как подключается SSO-провайдер

Подключение идёт через конфигурацию `auth`.

### OIDC-провайдер

Для OIDC доступны поля:

- `provider_name`
- `display_name`
- `base_url`
- `client_id`
- `client_secret`
- `extra_scopes`
- `allowed_domains`
- `field_map`
- `admin_mapping`
- `registration_enabled`
- `log_user_info`
- `log_sensitive_info`

### OAuth2-провайдер

Для plain OAuth доступны поля:

- `provider_name`
- `display_name`
- `client_id`
- `client_secret`
- `auth_url`
- `token_url`
- `user_info_url`
- `scopes`
- `allowed_domains`
- `field_map`
- `admin_mapping`
- `registration_enabled`
- `log_user_info`
- `log_sensitive_info`

### Критически важное требование

Для внешней авторизации обязательно корректно настроить:

- `web.external_url`

Именно на его основе строятся callback URL.

В коде callback path формируется как:

- `${external_url}${base_path}/api/v0/auth/login/{provider}/callback`

Если `external_url` настроен неверно, логин через SSO будет работать нестабильно или не будет работать вообще.

## 1.4. Жизненный цикл подключения провайдера при старте приложения

Во время запуска `Authenticator.StartBackgroundJobs()` поднимает внешние auth providers.

Что делает система:

1. читает конфигурацию `oidc`, `oauth`, `ldap`
2. пытается инициализировать каждый провайдер
3. если часть провайдеров не поднялась, они остаются в очереди retry
4. повторяет попытку инициализации каждые 30 секунд

Это важная деталь эксплуатации:

- сбой провайдера на старте не всегда фатален для всего приложения
- портал может “дожить” до восстановления внешнего IdP

### Ограничения инициализации

- `provider_name` должен быть уникальным
- повторная регистрация одинакового идентификатора не допускается
- ошибочная конфигурация конкретного провайдера не должна ломать остальные

## 1.5. Что видит пользователь в UI

Frontend запрашивает список внешних провайдеров через:

- `GET /api/v0/auth/providers`

Backend возвращает для каждого провайдера:

- `Identifier`
- `Name`
- `ProviderUrl`
- `CallbackUrl`

На login screen:

- рендерятся отдельные кнопки под каждый провайдер
- `display_name` может содержать HTML-разметку для кнопки
- пользователь выбирает провайдера и уходит во внешний redirect flow

### Скрытие локальной формы логина

Есть отдельная настройка:

- `auth.hide_login_form`

Если она включена:

- форма username/password скрывается
- пользователю остаются SSO / WebAuthn / внешние способы входа

При этом проект сознательно оставляет escape hatch:

- `#/login?all`

Этот параметр позволяет открыть полную форму логина даже при скрытой локальной форме.

## 1.6. Как работает SSO-flow технически

### Шаг 1. Инициация логина

Маршрут:

- `GET /api/v0/auth/login/{provider}/init`

Что происходит:

1. backend проверяет, что провайдер существует
2. генерирует `state`
3. для OIDC дополнительно генерирует `nonce`
4. строит URL авторизации у внешнего IdP
5. сохраняет в session:
   - `OauthState`
   - `OauthNonce`
   - `OauthProvider`
   - `OauthReturnTo`

Есть два режима:

- возврат JSON с `redirectUrl`
- немедленный redirect, если передан `redirect=true`

### Шаг 2. Redirect на IdP

Frontend в `LoginView.vue` формирует URL вида:

- `{backend}/auth/login/{provider}/init?redirect=true&return=<current frontend url>`

После этого браузер уходит на страницу логина внешнего провайдера.

### Шаг 3. Callback от провайдера

Маршрут:

- `GET /api/v0/auth/login/{provider}/callback`

Что проверяется:

- совпадение провайдера с тем, что был записан в session
- совпадение `state`
- корректность `return` URL

После этого backend вызывает `OauthLoginStep2`.

### Шаг 4. Обмен `code` на token

Зависит от типа провайдера:

- OIDC: `Exchange` + разбор и валидация `id_token`
- OAuth2: `Exchange` + запрос на `user_info_url`

### Шаг 5. Получение и парсинг user info

После получения raw claims / user info backend маппит их в внутреннюю модель:

- `Identifier`
- `Email`
- `Firstname`
- `Lastname`
- `Phone`
- `Department`
- `IsAdmin`
- `AdminInfoAvailable`

### Шаг 6. Регистрация или обновление пользователя

Если пользователь уже есть:

- обновляются внешние атрибуты
- при необходимости добавляется новый `AuthSource`

Если пользователя нет:

- при `registration_enabled=true` он будет создан автоматически
- при `registration_enabled=false` логин завершится ошибкой

### Шаг 7. Создание локальной web session

После успешного логина backend:

- очищает старую session
- пишет новую session
- сохраняет:
  - `LoggedIn`
  - `IsAdmin`
  - `UserIdentifier`
  - `Firstname`
  - `Lastname`
  - `Email`

Дальше frontend поднимает авторизованное состояние.

## 1.7. Специфика OIDC реализации

OIDC-реализация сильнее и безопаснее plain OAuth.

### Что делается правильно

- discovery через `oidc.NewProvider`
- проверка `id_token`
- проверка `ClientID` через verifier
- проверка `nonce`

### Что это даёт

- меньше ручной конфигурации
- выше надёжность identity assertions
- лучше подходит для корпоративных IdP

### Какие claims реально используются

Зависит от `field_map`, но по умолчанию:

- `user_identifier = sub`
- `email = email`
- `firstname = given_name`
- `lastname = family_name`
- `phone = phone`
- `department = department`
- `is_admin = admin_flag`
- `user_groups` по умолчанию не используется

## 1.8. Специфика plain OAuth2 реализации

Plain OAuth2 работает проще:

1. получает access token
2. отправляет `GET` на `user_info_url`
3. добавляет `Authorization: Bearer <token>`
4. разбирает JSON ответа

### Сильные стороны

- можно подключать провайдеры без полноценного OIDC
- высокая гибкость за счёт ручной конфигурации endpoint’ов

### Ограничения

- доверие к user info endpoint целиком на стороне интегратора
- меньше встроенной верификации, чем у OIDC
- точность зависит от правильного `field_map`

## 1.9. Маппинг атрибутов и преобразование пользователя

Портал поддерживает гибкий `field_map`.

### Какие поля можно маппить

- `user_identifier`
- `email`
- `firstname`
- `lastname`
- `phone`
- `department`
- `is_admin`
- `user_groups`

### Что это даёт practically

- можно адаптировать практически любой IdP
- можно использовать нестандартные custom claims
- можно подстраивать mapping под структуру конкретного провайдера

### Важный нюанс

Ключевой идентификатор пользователя определяется именно `user_identifier`.

Это означает:

- если выбран нестабильный claim, возможны дубли и расщепление identity
- если выбран стабильный claim, один и тот же пользователь будет корректно “сходиться” между логинами

Для OIDC safest choice обычно:

- `sub`

## 1.10. Ограничение доступа по доменам

И OIDC, и OAuth поддерживают:

- `allowed_domains`

Как это работает:

- после получения email система берёт доменную часть
- если список пуст, доступ открыт
- если список заполнен, домен email должен точно совпасть с allowlist

### Важные особенности

- сравнение case-insensitive
- нет wildcard-механизма
- проверка основана на email, а не на `hd`, tenant id или group membership

### Практический вывод

Это удобный базовый фильтр доступа, но не enterprise-grade policy engine.

## 1.11. Назначение админских прав из SSO

Портал умеет назначать `IsAdmin` из внешнего IdP.

Поддерживаются два механизма.

### 1. По claim `is_admin`

Через:

- `field_map.is_admin`
- `admin_mapping.admin_value_regex`

Если значение claim совпало с regex:

- пользователь становится администратором

### 2. По membership в группе

Через:

- `field_map.user_groups`
- `admin_mapping.admin_group_regex`

Если любой из элементов `user_groups` совпадает с regex:

- пользователь становится администратором

### Важная деталь реализации

`IsAdmin` обновляется только если `AdminInfoAvailable=true`.

Это правильно, потому что:

- если провайдер не отдаёт admin-информацию, система не затирает роль вслепую

## 1.12. Автоматическая регистрация и обновление пользователя

### Регистрация

Если `registration_enabled=true`:

- новый пользователь создаётся автоматически при первом успешном логине

Создаваемые данные:

- identifier
- email
- имя / фамилия
- телефон
- department
- admin-флаг, если он надёжно определён
- auth source с типом `oauth` и именем провайдера

### Обновление

Если пользователь уже существует:

- источник аутентификации добавляется в список, если его ещё не было
- email, имя, фамилия, телефон, department синхронизируются
- `IsAdmin` синхронизируется, если admin-информация доступна

### Флаг `PersistLocalChanges`

Если у пользователя включён `PersistLocalChanges`:

- внешние атрибуты не перезаписываются
- но новый auth source всё равно может быть добавлен

Это важное поведение для гибридных сценариев ручного и внешнего управления профилем.

## 1.13. Связь нескольких auth sources с одним пользователем

Проект умеет объединять несколько источников входа в один локальный user record.

Это работает через:

- единый `Identifier`
- список `Authentications`

Практический эффект:

- один и тот же пользователь может входить через локальную БД, LDAP и SSO
- в профиль добавляются новые auth sources
- учётная запись остаётся единой

### Риск этой модели

Если два разных внешних IdP используют одинаковый `user_identifier` для разных реальных людей:

- записи сольются в одного пользователя

Следовательно, выбор `field_map.user_identifier` критичен.

## 1.14. Ограничения и защитные проверки в SSO-flow

### Что проверяется

- existence провайдера
- уникальность имени провайдера на старте
- `state` в callback
- `nonce` для OIDC
- return URL должен совпадать по `scheme` и `host` с `external_url`
- user должен быть не locked и не disabled
- allowed domain policy

### Что важно отметить

Проверка `return` URL:

- сравнивает только `scheme` и `host`
- не делает сложной allowlist по path

Для большинства случаев этого достаточно, но модель не сверхжёсткая.

## 1.15. Отказоустойчивость и наблюдаемость SSO

Для SSO предусмотрены:

- retry инициализации провайдеров при старте
- trace/debug logging
- отдельные флаги `log_user_info`
- отдельные флаги `log_sensitive_info`
- audit events успешных и неуспешных логинов

### Что логируется опасно

Если включить `log_sensitive_info`:

- в логах могут оказаться токены и сырые ответы провайдера

Это пригодно только для временной диагностики.

## 1.16. Вывод по SSO-функционалу

SSO-функционал в проекте зрелый и практически применимый.

Он поддерживает:

- несколько внешних провайдеров одновременно
- OIDC и plain OAuth2
- авто-регистрацию
- авто-синхронизацию профиля
- внешнее назначение admin-ролей
- ограничение по доменам
- скрытие локального логина

Сильные стороны:

- хороший OIDC flow
- нормальная session-based web integration
- гибкий field mapping
- удобный multi-provider support

Основные ограничения:

- точность identity сильно зависит от `field_map.user_identifier`
- domain allowlist построен только на email
- plain OAuth заведомо слабее OIDC по уровню доверия

---

## 2. Функционал SCIM

## 2.1. Общая картина

В ветке `feature/SCIM_deprovisioning` реализован SCIM v2.0 endpoint для управления пользователями.

SCIM включается конфигурацией:

- `scim.enabled`
- `scim.bearer_token`
- `scim.delete_action`

То есть SCIM в проекте:

- не всегда активен
- не публичен “по умолчанию”
- настраивается как отдельный enterprise integration channel

## 2.2. Где и как монтируется SCIM

SCIM handler монтируется в HTTP server под:

- `/scim/`

Используется `http.StripPrefix`, поэтому библиотечный SCIM server обслуживает стандартные SCIM-маршруты внутри этого префикса.

Практически это означает доступность путей вида:

- `/scim/v2/Users`
- `/scim/v2/Users/{id}`
- `/scim/v2/Schemas`
- `/scim/v2/ResourceTypes`
- `/scim/v2/ServiceProviderConfig`

## 2.3. Аутентификация SCIM

SCIM использует:

- `Bearer Token`

### Как это реализовано

Middleware:

- читает `Authorization`
- требует префикс `Bearer `
- сравнивает токен через `subtle.ConstantTimeCompare`

Если токен неверен:

- возвращается `401`
- content-type: `application/scim+json`
- тело SCIM-compatible error

Если токен верен:

- request переводится в `SystemAdmin` context

### Что это означает

Все SCIM-операции исполняются как системный администратор, а значит:

- SCIM не зависит от web session
- SCIM не зависит от local user session lifecycle
- SCIM имеет полный административный доступ к user management

## 2.4. Что умеет SCIM server

В реализации зарегистрирован один resource type:

- `User`

Поддерживаются операции:

- `Create`
- `Get`
- `GetAll`
- `Replace`
- `Patch`
- `Delete`

Также библиотека автоматически даёт служебные SCIM endpoints:

- `Schemas`
- `ResourceTypes`
- `ServiceProviderConfig`

## 2.5. Поддерживаемая схема пользователя

В схеме `User` определены атрибуты:

- `userName`
- `displayName`
- `active`
- `name.givenName`
- `name.familyName`
- `name.formatted` как read-only
- `emails[].value`
- `emails[].type`
- `emails[].primary`
- `phoneNumbers[].value`
- `phoneNumbers[].type`

Из кода обработчика дополнительно видно, что поддерживается:

- `externalId`

То есть фактическая модель интеграции включает ещё и привязку внешнего идентификатора SCIM-системы.

## 2.6. Маппинг SCIM -> внутренний `domain.User`

### Из SCIM в доменную модель

Поля маппятся так:

- `userName` -> `Identifier`
- `externalId` -> `ExternalId`
- `name.givenName` -> `Firstname`
- `name.familyName` -> `Lastname`
- `emails` -> `Email`
- `phoneNumbers` -> `Phone`
- `active=false` -> `Disabled = now`

### Из доменной модели в SCIM

При отдаче ресурса обратно:

- `ID = Identifier`
- `userName = Identifier`
- `active = (Disabled == nil)`
- `displayName = DisplayName()`
- `name.givenName = Firstname`
- `name.familyName = Lastname`
- `emails[0] = Email`
- `phoneNumbers[0] = Phone`
- `externalId = ExternalId`
- `meta.created`
- `meta.lastModified`

## 2.7. Реально поддерживаемые CRUD-сценарии

### Create

SCIM может создать пользователя.

Поведение:

- собирает `domain.User` из SCIM attributes
- вызывает `CreateUser`
- при дубликате возвращает `409 uniqueness`

### Get

SCIM может получить пользователя по ID.

Поведение:

- ищет по `Identifier`
- при отсутствии возвращает `404`

### GetAll

SCIM может получить список пользователей.

Поддерживается:

- фильтрация через `FilterValidator`
- пагинация через `startIndex` и `count`

### Replace

Полная замена пользователя через `PUT`.

Поведение:

- строится новый `domain.User`
- `Identifier` принудительно берётся из path
- вызывается `UpdateUser`

### Patch

Поддерживается частичное обновление.

Реализованы операции:

- `add`
- `replace`
- `remove`

Поддерживаемые patch path:

- `active`
- `userName`
- `name.givenName`
- `name.familyName`
- `externalId`
- `emails`
- `phoneNumbers`

### Delete

Удаление зависит от конфигурации `scim.delete_action`.

Это центральная функциональность данной ветки.

## 2.8. Deprovisioning: ключевой функционал ветки

### Режим `disable`

Это поведение по умолчанию.

Что происходит:

- пользователь не удаляется из БД
- у него выставляется `Disabled = now`
- `DisabledReason = "SCIM deprovisioned"`

Практический смысл:

- сохраняется audit trail
- сохраняются связи и история
- учётка деактивируется мягко

### Режим `delete`

В этом режиме:

- вызывается `DeleteUser`
- пользователь удаляется физически

### Почему это важно

Для enterprise IAM это два разных класса интеграции:

- soft deprovisioning
- hard delete

Ветка добавляет возможность выбирать поведение явно.

## 2.9. PATCH и управление активностью

Через SCIM PATCH уже можно управлять активностью пользователя.

### `active = false`

Поведение:

- пользователь деактивируется
- выставляется `Disabled`

### `active = true`

Поведение:

- `Disabled` очищается
- `DisabledReason` очищается

Это значит, что SCIM уже поддерживает lifecycle:

- activate
- deactivate
- patch profile fields

## 2.10. Фильтрация и пагинация SCIM

В `GetAll` реализовано:

- прохождение SCIM filter validator по каждому ресурсу
- ручная пост-фильтрация списка
- пагинация по `startIndex` и `count`

### Практический эффект

SCIM endpoint уже пригоден для:

- инкрементального чтения
- синхронизации внешним IAM
- точечного поиска через filter

### Ограничение реализации

Фильтрация происходит:

- не на уровне БД
- а в памяти после чтения всех пользователей

Следствие:

- для маленьких и средних инсталляций это нормально
- для очень больших инсталляций это может стать узким местом

## 2.11. Границы функциональности SCIM

### Что точно поддерживается

- только ресурс `Users`
- CRUD + PATCH
- bearer token auth
- deactivate vs delete
- `externalId`
- SCIM-compatible error responses

### Чего в этой реализации нет

- групп SCIM
- SCIM для peer’ов или интерфейсов
- сложной SCIM authorization model кроме bearer token
- batch endpoints
- ETag / versioning semantics на уровне ресурса
- оптимизированной server-side фильтрации по БД

То есть текущая реализация ориентирована именно на user provisioning lifecycle.

## 2.12. Тестовое покрытие SCIM

Ветка содержит отдельные тесты, которые подтверждают:

- отказ без bearer token
- отказ с неправильным токеном
- успешный доступ с валидным токеном
- create user
- duplicate handling
- get user
- get all users
- filter
- replace
- patch deactivate
- delete как disable
- delete как hard delete
- round-trip mapping user <-> resource
- корректный выбор primary email

Это хороший знак: SCIM добавлен не только кодом, но и минимально верифицирован тестами.

## 2.13. Практическая применимость SCIM в текущем состоянии

Текущий SCIM функционал уже подходит для интеграций с системами класса:

- Okta
- Azure / Entra provisioning
- Keycloak / IAM sync
- custom HR / IAM lifecycle systems

Поддерживаемые сценарии:

- автоматическое создание пользователя
- обновление профиля
- деактивация пользователя
- физическое удаление пользователя
- чтение пользователей обратно во внешний IAM

## 2.14. Ограничения и риски SCIM-реализации

### 1. Только users

SCIM покрывает только user lifecycle, без групп и без VPN-объектов.

### 2. Delete через `disable` не удаляет запись

Это часто плюс, но внешняя система может ожидать жёсткое удаление.

Значит:

- integrator должен осознанно выбрать `delete_action`

### 3. PATCH меняет `userName`

В `applyPatchPath` поддерживается изменение `userName`, то есть внутреннего `Identifier`.

Это потенциально чувствительная операция, потому что:

- `Identifier` является ключевым identity field
- переименование может повлиять на связность данных и внешние интеграции

### 4. Фильтрация не на БД

При росте числа пользователей возможен overhead при `GetAll`.

### 5. Bearer token один на весь SCIM access

Модель простая и рабочая, но:

- нет разграничения прав по операциям
- нет rotate / multi-token модели внутри приложения

## 2.15. Вывод по SCIM

SCIM в этой ветке реализован как прикладной enterprise provisioning endpoint для пользователей.

Сильные стороны:

- стандартный SCIM v2.0 server behavior
- bearer token auth
- CRUD + PATCH
- `externalId`
- deprovisioning через `disable` или `delete`
- наличие тестов

Главная функциональная ценность ветки:

- система теперь может полноценно встраиваться в enterprise identity lifecycle и поддерживать controlled deprovisioning

---

## Итог

### По SSO

В проекте реализован полноценный и практичный SSO-слой:

- multi-provider
- OIDC
- OAuth2
- внешнее назначение ролей
- auto-registration
- field mapping
- domain restrictions
- web session integration

Это уже production-usable функционал при условии аккуратной настройки `external_url`, `field_map` и provider claims.

### По SCIM

Ветка `feature/SCIM_deprovisioning` добавляет отдельный enterprise integration layer:

- SCIM user provisioning
- soft deprovisioning
- hard delete
- bearer-token protected admin access

С точки зрения продукта именно SCIM в этой ветке является главным новым функциональным усилением вокруг авторизации и identity lifecycle.
