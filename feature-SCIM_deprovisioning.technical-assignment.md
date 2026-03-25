# Техническое задание: развитие SSO, SCIM, identity lifecycle и конфигурации `wg-portal`

## 1. Основание для подготовки ТЗ

ТЗ подготовлено на основе следующих документов:

- `feature-SCIM_deprovisioning.md`
- `feature-SCIM_deprovisioning.authorization.md`
- `feature-SCIM_deprovisioning.optimization.md`

А также на основе анализа текущей реализации в коде проекта.

## 2. Цель работ

Разработать и внедрить следующий набор изменений:

1. усилить и формализовать SSO и SCIM-интеграции
2. ввести управляемый identity lifecycle
3. расширить ролевую модель и управление доступом по группам
4. перевести рабочую конфигурацию приложения на хранение в базе данных
5. реализовать полнофункциональную настройку `wg-portal` через Web UI
6. обеспечить контролируемое применение конфигурации из файла только в специальном режиме `--reconfigure`

## 3. Текущее состояние и ограничения

### 3.1. По авторизации и ролям

Сейчас проект поддерживает:

- локальную аутентификацию
- LDAP
- OIDC
- OAuth2
- WebAuthn

Текущая ролевая модель фактически бинарная:

- `Admin`
- `User`

Назначение ролей из внешних источников сейчас покрывает только admin mapping и не покрывает отдельные доменные роли, например:

- `monitoring`
- расширенные пользовательские группы

Доступ к интерфейсам по группам частично реализован только через:

- `LDAP interface_filter`

Для OIDC/OAuth аналогичная модель допуска к интерфейсам в текущем виде отсутствует.

### 3.2. По SCIM

Сейчас SCIM:

- поддерживает только `Users`
- поддерживает CRUD и PATCH
- использует bearer token
- поддерживает `delete_action = disable|delete`

При этом:

- изменение `userName` через PATCH возможно
- фильтрация `GetAll` выполняется в памяти, не на уровне БД
- нет расширенной модели token rotation
- нет SCIM Groups

### 3.3. По конфигурации приложения

Сейчас рабочая конфигурация приложения загружается из:

- конфигурационного файла
- environment variables

Текущая модель хранения конфигурации в БД как primary runtime source отсутствует.

### 3.4. По Web UI конфигурированию

Сейчас в Web UI отсутствует полноценная административная подсистема настройки всех параметров `wg-portal`.

Имеются только отдельные пользовательские и эксплуатационные экраны:

- профиль
- пользовательские настройки
- аудит
- управление интерфейсами, peer’ами, пользователями

Полного покрытия параметров `config.yml` через Web UI на текущий момент нет.

## 4. Границы ТЗ

В состав работ входят:

- backend
- API
- схема и хранение конфигурации
- миграции
- SCIM
- SSO
- UI для администрирования конфигурации
- документация
- аудит
- диагностические и preflight-механизмы

В состав работ не входят:

- внедрение стороннего IAM
- миграция внешних корпоративных систем
- рефакторинг всех остальных подсистем, не затронутых данным ТЗ, без явной необходимости

## 5. Целевая модель ролей и доступа

## 5.1. Требуемые роли

Авторизация и доступ должны покрывать следующие кейсы:

- назначение ролей согласно группам
- доступ к интерфейсам согласно группам

Минимально обязательные роли:

- `admin`
- `user`
- `monitoring`

### Назначение ролей

Роли должны назначаться на основе membership/group mapping из внешних источников:

- OIDC
- OAuth2
- LDAP
- в перспективе SCIM Groups

### Назначение доступа к интерфейсам

Доступ к интерфейсам должен уметь определяться согласно группам для:

- LDAP
- OIDC
- OAuth2
- в перспективе SCIM Groups

## 5.2. Семантика ролей

### `admin`

Имеет полный административный доступ:

- пользователи
- peer’ы
- интерфейсы
- конфигурация системы
- аудит
- SCIM/SSO diagnostics

### `user`

Имеет обычный пользовательский доступ:

- собственные peer’ы
- self-provisioning в рамках разрешённых интерфейсов
- собственные настройки

### `monitoring`

Имеет доступ только на чтение к наблюдаемости и эксплуатационным данным:

- метрики
- health/status провайдеров
- audit read-only
- диагностические экраны

Роль `monitoring` не должна иметь права:

- изменять конфигурацию
- изменять пользователей
- изменять peer’ы
- изменять интерфейсы

## 5.3. Требования к реализации ролевой модели

Необходимо:

1. отказаться от бинарной модели `IsAdmin` как единственного признака авторизации
2. ввести расширяемую role model
3. поддержать хранение набора ролей у пользователя
4. реализовать policy layer для проверки доступа по ролям
5. обеспечить обратную совместимость:
   - существующий `IsAdmin=true` должен мигрировать в роль `admin`
   - существующий `IsAdmin=false` должен мигрировать в роль `user`

## 6. Требования к конфигурации приложения

## 6.1. Новая приоритетная модель хранения конфигурации

Необходимо реализовать следующую модель:

1. рабочая конфигурация хранится в базе данных
2. приложение запускается только с параметрами конфигурации из БД
3. конфигурация через Web UI после валидации записывается в БД
4. применение конфигурации из файла конфигурации осуществляется только при запуске с флагом `--reconfigure`

## 6.2. Поведение `--reconfigure`

При запуске приложения с флагом `--reconfigure` требуется следующая логика:

1. считать конфигурацию из файла
2. выполнить полную валидацию
3. записать конфигурацию в БД
4. не запускать приложение в обычном runtime-режиме
5. вывести сообщение:
   - результат валидации
   - результат обновления конфигурации в БД

### Условия завершения режима `--reconfigure`

- при успехе приложение должно завершаться с кодом `0`
- при ошибке валидации или сохранения конфигурации приложение должно завершаться с ненулевым кодом

## 6.3. Старт приложения без `--reconfigure`

При обычном запуске:

1. файл конфигурации не должен быть источником рабочей runtime-конфигурации
2. приложение должно читать рабочую конфигурацию только из БД
3. при отсутствии конфигурации в БД должен быть реализован контролируемый сценарий:
   - либо старт запрещается с понятной ошибкой
   - либо разрешается bootstrap-режим по явно описанной политике

Предпочтительный вариант:

- старт запрещается до первичной загрузки конфигурации через `--reconfigure`

## 6.4. Требования к модели хранения конфигурации

Конфигурация в БД должна:

- покрывать все параметры текущего `config.yml`
- хранить version / revision
- поддерживать audit trail изменений
- поддерживать валидацию до активации
- поддерживать безопасное хранение секретов

Требуется спроектировать:

- схему хранения
- модель версионирования
- модель активной конфигурации
- модель черновика и опубликованной версии

Минимально:

- `draft`
- `active`

Предпочтительно:

- `draft`
- `validated`
- `active`
- `history`

## 6.5. Настройка через Web UI

Необходимо проверить и реализовать возможность настройки `wg-portal` через Web UI.

Результат должен покрывать все параметры, задающиеся через файл конфигурации.

Это требование является обязательным.

### UI должен покрывать как минимум:

- `core`
- `advanced`
- `web`
- `database` в части допустимой для runtime и безопасности
- `mail`
- `webhook`
- `auth`
- `oidc`
- `oauth`
- `ldap`
- `webauthn`
- `backend`
- `statistics`
- `scim`

### Для Web UI конфигурации обязательно:

- form validation
- server-side validation
- draft/save/apply semantics
- контроль прав доступа
- audit записи по изменению конфигурации
- понятные сообщения об ошибках

## 7. Функциональные требования

## FR-1. Полностью запретить изменение `userName` через SCIM PATCH и PUT

### Требование

Необходимо полностью запретить изменение `userName` через:

- SCIM `PATCH`
- SCIM `PUT`

### Детализация

- path `userName` должен считаться immutable
- при попытке изменить `userName` сервис должен возвращать корректную SCIM error response
- необходимо обновить документацию SCIM
- необходимо покрыть тестами:
  - PATCH attempt
  - PUT attempt с несовпадающим `userName`
  - сценарий, когда `id` в path и `userName` в payload различаются

### Критерий приёмки

Любая попытка изменить `userName` через SCIM завершается отказом и не меняет запись пользователя.

## FR-2. Добавить режим обязательного OIDC вместо plain OAuth там, где это возможно

### Требование

Необходимо формализовать OIDC как предпочтительный и рекомендуемый enterprise path.

### Подзадачи

1. явно рекомендовать OIDC как default enterprise path
2. в документации и конфиг-примерах маркировать plain OAuth как fallback
3. добавить startup warning, если настроен OAuth provider при наличии OIDC-аналога

### Дополнительные требования

- реализовать policy/flag, позволяющий требовать использование OIDC в enterprise-режиме
- определить, что считается “OIDC-аналогом” для предупреждения
- warning должен быть структурированным и читаемым

### Критерий приёмки

- документация обновлена
- при startup портал предупреждает об использовании plain OAuth как fallback path
- для enterprise policy возможно запретить plain OAuth

## FR-3. Ужесточить валидацию `external_url` и callback-конфигурации на старте

### Требование

При наличии `oidc` / `oauth` необходимо выполнять строгую проверку:

- `external_url` задан
- `external_url` корректно парсится
- `external_url` не пустой
- `http` используется только в допустимых local / lab сценариях
- callback URL не содержит очевидных противоречий

### Обязательные проверки

При наличии `oidc` / `oauth` выдавать warning или hard error, если:

- `external_url` пустой
- `external_url` использует `http` в non-local setup
- callback URL очевидно неконсистентен

### Требование к политике строгости

Необходимо разделить:

- `warning`
- `error`

Примерно так:

- local `http://localhost` допустим с warning/без warning
- public hostname + `http` должен быть error или строгое предупреждение по policy

## FR-4. Добавить SCIM audit trail как отдельный источник событий

### Требование

Необходимо ввести отдельный источник аудита для SCIM.

### Должны аудироваться

- SCIM authentication failure
- SCIM create
- SCIM read list/get при необходимости по policy
- SCIM replace
- SCIM patch
- SCIM disable
- SCIM delete
- SCIM token management operations

### Поля аудита

- source = `scim`
- operation
- actor type
- target entity
- target identifier
- request id / correlation id
- result
- reason / error

## FR-5. Сделать SCIM deprovisioning более явным и расширяемым

### Требование

Текущий `delete_action = disable|delete` необходимо расширить до управляемой deprovisioning policy.

### Минимально обязательные варианты

- `disable`
- `delete`
- `lock`
- `disable_and_revoke_api`
- `disable_and_disable_peers`

### Дополнительно

Реализовать policy flags:

- deprovision reason
- disable peers on deprovision
- delete peers on deprovision
- revoke tokens / sessions on deprovision

### Критерий приёмки

SCIM deprovisioning должен быть описываемым политикой и тестируемым по каждому режиму.

## FR-6. Вынести SSO user mapping в отдельный reusable normalization layer

### Требование

Необходимо выделить reusable слой нормализации внешней identity-информации.

### Новый слой должен отвечать за

- нормализацию claims
- mapping внешних полей
- нормализацию email
- вычисление ролей
- интерпретацию групп
- вычисление доступа к интерфейсам
- валидацию обязательных полей

### Источники данных

- OIDC
- OAuth2
- LDAP
- в перспективе SCIM

## FR-7. Добавить server-side фильтрацию SCIM на уровне БД

### Требование

SCIM `GetAll` должен по возможности фильтроваться на стороне БД.

### Минимально поддерживаемые фильтры на стороне БД

- `userName eq`
- `externalId eq`
- `emails.value eq`
- `active eq`

### Обязательное поведение

- для простых фильтров использовать DB query
- для неподдерживаемых фильтров использовать fallback strategy
- пагинация должна быть корректной

## FR-8. Добавить health/status модель для auth providers

### Требование

Нужно реализовать runtime status model для auth providers.

### Для каждого провайдера отображать

- provider id
- provider type
- init status
- availability status
- last error
- last successful check
- effective callback URL

### Поверхности отображения

- diagnostic endpoint
- admin UI
- при необходимости metrics

## FR-9. Добавить preflight endpoint для проверки SSO-конфигурации

### Требование

Нужно реализовать endpoint для административной проверки SSO-конфигурации без реального логина конечного пользователя.

### Endpoint должен проверять

- OIDC discovery
- token endpoint
- user info endpoint
- callback consistency
- field mapping consistency
- role mapping consistency
- interface access mapping consistency
- allowed_domains policy

### Формат результата

- structured JSON
- статус `ok|warning|error`
- список найденных проблем
- список рекомендаций

## FR-10. Добавить явную политику trust-level для внешних admin mapping

### Требование

Необходимо формализовать политику доверия к внешнему назначению ролей.

### Нужно реализовать

- trust-level policy для role mapping
- отдельную политику для:
  - claim-based mapping
  - group-based mapping
- настройку степени доверия к источнику

### Минимальные режимы

- disabled
- claim
- group
- claim_or_group
- claim_and_group

### Расширение на новые роли

Политика должна работать не только для `admin`, но и для:

- `user`
- `monitoring`

## FR-11. Ограничить HTML в `display_name` провайдера

### Требование

Поддерживать только plain text в `display_name`.

### Следствия

- произвольный HTML в `display_name` должен быть запрещён
- frontend должен отображать только текст
- migration strategy должна очистить/экранировать существующие значения

## FR-12. Добавить rotation и multi-token модель для SCIM

### Требование

Нужно заменить single bearer token модель на управляемую multi-token model.

### Поддержать

- несколько активных SCIM tokens
- имя/описание токена
- дата создания
- expiration
- revoke
- rotation
- optional overlap window

### Хранение

- только в безопасном виде
- без хранения токенов в plaintext, если это возможно по выбранной модели

## FR-13. Добавить rate limiting для auth и SCIM endpoints

### Требование

Необходимо добавить rate limiting для:

- plain login
- SSO init
- SSO callback
- SCIM endpoints
- SCIM auth failures

### Rate limiting должен быть:

- конфигурируемым
- наблюдаемым
- с безопасным fail behavior

## FR-14. Отдельно нормализовать и верифицировать email

### Требование

Email из внешних источников должен проходить отдельный normalization/verification pipeline.

### Обязательные шаги

- lowercase normalization
- trimming
- validation формата
- optional `email_verified` policy для OIDC/OAuth
- policy на обязательность email

### Дополнительное требование

Результат нормализации должен использоваться в:

- role mapping
- domain restriction
- user merge strategy

## FR-15. Показывать итоговый resolved mapping в админском UI/diagnostics

### Требование

Для каждого auth provider нужно показывать итоговый resolved mapping.

### Должно быть видно

- raw claims / attributes по тестовому примеру или probe
- resolved identifier
- resolved email
- resolved roles
- resolved interface access groups
- matched/unmatched policies

## FR-16. Улучшить тексты ошибок SSO для администратора и пользователя

### Требование

Нужно разделить:

- user-facing ошибки
- admin-facing diagnostic ошибки

### Примеры ошибок

- provider not initialized
- invalid state
- invalid nonce
- registration disabled
- role mapping failed
- group mapping failed
- email not verified
- email domain not allowed
- callback mismatch

## FR-17. Добавить SCIM capability section в основную документацию

### Требование

Нужно оформить SCIM как полноценный documented capability.

### Документация должна содержать

- включение SCIM
- token management
- endpoints
- schema
- delete/deprovision policy
- limitations
- примеры запросов
- примеры интеграции

## FR-18. Поддержать SCIM Groups

### Требование

Нужно поддержать SCIM Groups как отдельный этап развития identity model.

### Поддержка групп требуется для

- массового назначения ролей
- управления доступом к интерфейсам
- синхронизации организационной структуры

### Минимальный объём первой версии

- чтение групп
- привязка групп к пользователю
- group-to-role mapping
- group-to-interface-access mapping

## FR-19. Свести SSO, LDAP и SCIM в единый identity lifecycle layer

### Требование

Необходимо спроектировать единый identity lifecycle layer, объединяющий:

- SSO
- LDAP
- SCIM

### Он должен отвечать за

- create/update/disable/delete
- merge policy
- source attribution
- role assignment
- interface access assignment
- audit trail

## FR-20. Ввести явную source precedence policy

### Требование

Нужно ввести configurable policy приоритетов источников данных.

### Источники

- local
- ldap
- sso
- scim

### Поля, для которых приоритет должен быть управляемым

- email
- firstname
- lastname
- phone
- department
- roles
- interface access
- disabled state

## 8. Дополнительные обязательные проверки и корректировки ТЗ

## 8.1. Роли и группы

ТЗ должно явно покрывать следующие кейсы.

### Назначение ролей согласно группам

Обязательно должны поддерживаться:

- `администраторы`
- `пользователи`
- `мониторинг`

### Доступ к интерфейсам согласно группам

Нужно поддержать назначение access policy к интерфейсам на основе групп:

- для LDAP
- для OIDC/OAuth
- для SCIM Groups на целевой стадии

### Следствие для реализации

Необходимо проектировать не только role mapping, но и access mapping layer:

- role group mapping
- interface access group mapping

## 8.2. Приоритет, порядок, хранение конфигурации

Уточнение включается в ТЗ как обязательное требование.

### Должно быть реализовано

- рабочая конфигурация хранится в БД
- приложение запускается только с конфигурацией из БД
- конфигурация через Web UI после валидации записывается в БД
- конфигурация из файла применяется только через `--reconfigure`
- при `--reconfigure` старт runtime не производится

## 8.3. Проверка и реализация настройки через Web UI

Уточнение включается в ТЗ как обязательное.

### Нужно проверить и реализовать

- возможность настройки `wg-portal` через Web UI
- покрытие всех параметров, задающихся через файл конфигурации

### Итоговое требование

Административный Web UI должен стать primary interface для управления конфигурацией приложения.

## 9. Нефункциональные требования

## NFR-1. Безопасность

- секреты не должны раскрываться в UI, логах и API без необходимости
- все изменения конфигурации должны аудироваться
- HTML injection через provider display name должен быть исключён
- token rotation должна быть безопасной

## NFR-2. Наблюдаемость

- все auth/scim/config lifecycle изменения должны быть видны в audit
- нужны health/status endpoints
- нужны структурированные startup warnings/errors

## NFR-3. Производительность

- SCIM list/filter должен масштабироваться лучше текущей in-memory модели
- diagnostics и preflight не должны блокировать основной runtime

## NFR-4. Обратная совместимость

- существующие пользователи и роли должны мигрировать без потери доступа
- существующие auth providers должны мигрировать в новую модель конфигурации
- существующий `config.yml` должен оставаться bootstrap source только для `--reconfigure`

## 10. Миграции

Нужно предусмотреть миграции:

1. пользователей из `IsAdmin` в role set
2. конфигурации из файла в БД
3. single SCIM token -> token set
4. auth providers -> новая конфигурационная модель
5. interface access policy -> group-based model

## 11. API и UI deliverables

## API

Должны быть добавлены/изменены:

- SCIM token management API
- auth provider status API
- SSO preflight API
- configuration CRUD API
- draft/validate/apply configuration API
- role and interface access mapping API

## UI

Должны быть добавлены:

- экран управления системной конфигурацией
- экран управления auth providers
- экран управления role mapping
- экран управления interface access mapping
- экран SCIM token management
- экран diagnostics / resolved mapping / provider health

## 12. Документация

Нужно обновить:

- конфигурационную документацию
- документацию SSO
- документацию SCIM
- примеры конфигурации
- описание режима `--reconfigure`
- описание новой модели хранения конфигурации в БД
- админскую документацию по Web UI configuration

## 13. Критерии приёмки верхнего уровня

Работы считаются принятыми, если одновременно выполняются все условия:

1. SCIM не позволяет менять `userName`
2. OIDC оформлен как основной enterprise path, plain OAuth описан как fallback
3. startup validation корректно валидирует `external_url` и callback-конфигурацию
4. SCIM имеет отдельный audit trail
5. deprovisioning реализован политиками
6. SSO mapping вынесен в reusable normalization layer
7. SCIM filtering использует DB-side filtering для поддерживаемых сценариев
8. есть health/status модель auth providers
9. есть preflight endpoint для SSO
10. есть trust-level policy для role mapping
11. `display_name` провайдера поддерживает только plain text
12. SCIM token model поддерживает rotation и несколько токенов
13. auth и SCIM endpoints защищены rate limiting
14. email нормализуется и при необходимости верифицируется
15. в UI/diagnostics виден resolved mapping
16. улучшены user/admin error messages
17. SCIM описан в основной документации
18. поддержаны SCIM Groups
19. SSO, LDAP и SCIM сведены в единый identity lifecycle layer
20. введена source precedence policy
21. роли по группам покрывают `admin`, `user`, `monitoring`
22. доступ к интерфейсам назначается по группам
23. рабочая конфигурация хранится в БД
24. старт приложения без `--reconfigure` идёт только по конфигурации из БД
25. Web UI покрывает настройку всех параметров, задаваемых через файл

## 14. Предлагаемая этапность реализации

## Этап 1. Безопасность и стабилизация

- FR-1
- FR-2
- FR-3
- FR-4
- FR-11
- FR-13
- FR-14
- FR-16

## Этап 2. Базовый identity refactor

- FR-6
- FR-10
- FR-19
- FR-20
- расширенная ролевая модель
- доступ к интерфейсам по группам

## Этап 3. SCIM enterprise expansion

- FR-5
- FR-7
- FR-12
- FR-17
- FR-18

## Этап 4. Конфигурация в БД и Web UI

- новая config storage model
- `--reconfigure`
- configuration CRUD API
- Web UI для всех параметров
- audit и versioning конфигурации

## 15. Итог

Настоящее ТЗ фиксирует переход `wg-portal` от набора разрозненных механизмов внешней аутентификации и конфигурирования к единой модели:

- identity lifecycle platform
- role/group driven access control
- SCIM-ready enterprise provisioning
- DB-backed runtime configuration
- Web UI как основной инструмент администрирования

Документ должен использоваться как основной baseline для декомпозиции на epics, milestones и backlog задач.
