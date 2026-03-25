# Предложения по оптимизации: SSO и SCIM в `wg-portal`

## Контекст

Документ подготовлен по результатам анализа ветки `feature/SCIM_deprovisioning` и сфокусирован на двух областях:

- SSO через внешних провайдеров
- SCIM provisioning / deprovisioning

Ниже приведены практические предложения по оптимизации. Они сгруппированы по приоритету и ориентированы на:

- безопасность
- надёжность
- эксплуатацию
- масштабируемость
- удобство интеграции

## Приоритет 1. Самые полезные улучшения

## 1. Запретить изменение `userName` через SCIM PATCH без явной политики

### Проблема

Сейчас SCIM PATCH поддерживает изменение:

- `userName`

Во внутренней модели это фактически:

- ключевой `Identifier`

Для identity-систем это опасная операция, потому что:

- может сломать внешние привязки
- может осложнить поиск пользователя в связанных сущностях
- повышает риск неконсистентности между IAM и локальной БД

### Предложение

Сделать одно из двух:

1. полностью запретить изменение `userName` через PATCH и PUT
2. разрешать это только через отдельный конфиг-флаг, например `scim.allow_username_change`

### Ожидаемый эффект

- снижение риска повреждения identity linkage
- более предсказуемое поведение SCIM
- меньше неожиданных проблем в production

## 2. Добавить режим обязательного OIDC вместо plain OAuth там, где это возможно

### Проблема

Plain OAuth в текущем виде менее надёжен, чем OIDC:

- нет `id_token`
- нет встроенной криптографической проверки identity assertions
- доверие смещено на `user_info_url`

### Предложение

- явно рекомендовать OIDC как default enterprise path
- в документации и конфиг-примерах маркировать plain OAuth как fallback
- добавить startup warning, если настроен OAuth provider при наличии OIDC-аналога

### Ожидаемый эффект

- уменьшение вероятности некорректной SSO-интеграции
- более безопасные интеграции по умолчанию

## 3. Ужесточить валидацию `external_url` и callback-конфигурации на старте

### Проблема

Сейчас SSO критически зависит от:

- `web.external_url`

Но ошибка в этом значении обнаруживается только во время логина.

### Предложение

На старте приложения:

- валидировать, что `external_url` задан и корректно парсится
- при наличии `oidc` / `oauth` выдавать warning или hard error, если:
  - `external_url` пустой
  - `external_url` использует `http` в non-local setup
  - callback URL очевидно неконсистентен

### Ожидаемый эффект

- меньше runtime-сюрпризов
- быстрее диагностируются ошибки конфигурации

## 4. Добавить SCIM audit trail как отдельный источник событий

### Проблема

SCIM работает с правами system admin и меняет пользователей, но из анализа кода не видно отдельного, явно выраженного audit-слоя именно для SCIM-операций.

### Предложение

Явно логировать и аудировать:

- `SCIM create`
- `SCIM replace`
- `SCIM patch`
- `SCIM disable`
- `SCIM delete`
- `SCIM auth failure`

Желательно хранить:

- внешний request id
- actor type = `scim`
- operation
- target user
- delete mode (`disable` / `delete`)

### Ожидаемый эффект

- лучшее расследование инцидентов
- прозрачность lifecycle-изменений
- соответствие enterprise audit expectations

## 5. Сделать SCIM deprovisioning более явным и расширяемым

### Проблема

Сейчас есть два режима:

- `disable`
- `delete`

Но lifecycle-потребности обычно шире.

### Предложение

Расширить `scim.delete_action` в перспективе до моделей:

- `disable`
- `delete`
- `lock`
- `disable_and_revoke_api`
- `disable_and_disable_peers`

Дополнительно можно ввести:

- `scim.deprovision_reason`
- `scim.deprovision_disable_peers`
- `scim.deprovision_delete_peers`

### Ожидаемый эффект

- более точная интеграция с enterprise IAM
- меньше ручных post-processing действий после deprovisioning

## Приоритет 2. Существенные архитектурные улучшения

## 6. Вынести SSO user mapping в отдельный reusable normalization layer

### Проблема

Сейчас parsing и mapping SSO-данных разбросаны по:

- OIDC provider
- OAuth provider
- общему parser
- логике последующего `processUserInfo`

Это уже рабочая схема, но она плохо масштабируется при росте числа provider-specific особенностей.

### Предложение

Выделить отдельный слой, например:

- `IdentityNormalizer`
- `ExternalIdentityMapper`

Который будет отвечать за:

- нормализацию claims
- проверку обязательных полей
- валидацию `Identifier`
- нормализацию email
- интерпретацию admin/group mapping

### Ожидаемый эффект

- проще тестировать
- проще расширять новыми провайдерами
- меньше дублирования и скрытой логики

## 7. Добавить server-side фильтрацию SCIM на уровне БД

### Проблема

SCIM `GetAll` сейчас:

- получает всех пользователей
- фильтрует их в памяти
- потом пагинирует

Это нормально для небольших инсталляций, но плохо масштабируется.

### Предложение

Сделать двухуровневый подход:

- базовый SCIM filter parser -> маппинг на DB query для простых операторов
- fallback на in-memory filtering для сложных случаев

Минимум, что имеет смысл поддержать на уровне БД:

- `userName eq`
- `emails.value eq`
- `externalId eq`
- `active eq`

### Ожидаемый эффект

- лучшая производительность на больших инсталляциях
- меньшая нагрузка на память
- более предсказуемая работа внешних IAM систем

## 8. Добавить health/status модель для auth providers

### Проблема

Провайдеры инициализируются с retry, но нет явного админского представления:

- какой провайдер поднят
- какой не поднят
- какая была последняя ошибка

### Предложение

Добавить runtime status для auth providers:

- provider id
- type
- initialized / failed
- last error
- last successful check

Это можно вывести:

- в admin UI
- в diagnostic endpoint
- в startup logs более структурированно

### Ожидаемый эффект

- быстрее поддержка и диагностика SSO-инцидентов
- меньше ручного чтения логов

## 9. Добавить preflight endpoint для проверки SSO-конфигурации

### Проблема

Сейчас администратор узнаёт о проблеме интеграции в момент реального входа пользователя.

### Предложение

Сделать административный preflight / dry-run check:

- проверить discovery для OIDC
- проверить доступность token endpoint
- проверить userinfo endpoint
- показать итоговый callback URL
- показать валидность `field_map`
- показать warning по `allowed_domains`

### Ожидаемый эффект

- быстрее настройка
- меньше итераций “сохранил конфиг -> попробовал логин”

## 10. Добавить явную политику trust-level для внешних admin mapping

### Проблема

Сейчас `IsAdmin` может назначаться внешним провайдером через:

- regex по claim
- regex по группе

Это гибко, но в production полезно явно фиксировать trust-политику.

### Предложение

Добавить конфиг-уровень вроде:

- `admin_mapping_mode: disabled | claim | group | claim_or_group`

И отдельные защитные warnings:

- если настроен `is_admin`, но regex слишком общий
- если `user_groups` пустой, а `admin_group_regex` задан

### Ожидаемый эффект

- меньше ошибочных назначений admin-ролей
- выше прозрачность security-модели

## Приоритет 3. Улучшения безопасности

## 11. Ограничить HTML в `display_name` провайдера

### Проблема

Из примеров видно, что `display_name` используется в UI и может содержать HTML-разметку.

Это удобно для оформления кнопок, но:

- создаёт лишний surface для XSS/markup abuse

### Предложение

Вместо произвольного HTML:

- поддерживать только plain text
- либо whitelist очень ограниченных тегов

### Ожидаемый эффект

- меньше рисков во frontend
- более безопасное конфигурирование провайдеров

## 12. Добавить rotation и multi-token модель для SCIM

### Проблема

Сейчас SCIM использует:

- один bearer token на всё

Это просто, но неудобно для enterprise эксплуатации.

### Предложение

Поддержать:

- несколько SCIM tokens
- имя / описание токена
- дата создания
- статус active/revoked
- optional expiration

Минимальная версия:

- current token + next token в период ротации

### Ожидаемый эффект

- безопаснее rotate credentials
- проще интеграции с внешними IAM системами

## 13. Добавить rate limiting для auth и SCIM endpoints

### Проблема

Из текущего анализа не видно явного встроенного rate limiting для:

- login endpoints
- callback endpoints
- SCIM endpoints

### Предложение

Добавить rate limiting отдельно для:

- plain login
- SSO init/callback
- SCIM auth failures

### Ожидаемый эффект

- защита от brute force и abuse
- более контролируемая нагрузка

## 14. Отдельно нормализовать и верифицировать email

### Проблема

`allowed_domains` завязаны на email, но качество email полностью зависит от внешнего провайдера.

### Предложение

Добавить:

- нормализацию email до lowercase
- optional requirement `email must be present`
- optional requirement `email_verified=true` для OIDC-провайдеров, если claim доступен

### Ожидаемый эффект

- надёжнее domain restriction
- меньше проблем с дубликатами и mismatch

## Приоритет 4. Улучшения UX и поддержки интегратора

## 15. Показывать итоговый resolved mapping в админском UI/diagnostics

### Проблема

Интегратор часто не понимает:

- какой claim реально приходит
- во что он маппится
- почему не сработал admin mapping

### Предложение

Добавить diagnostic view:

- raw fields preview
- resolved identifier
- resolved email
- resolved role decision
- matched / unmatched allowed_domains

### Ожидаемый эффект

- сильно ускоряет настройку SSO
- уменьшает необходимость включать sensitive logging

## 16. Улучшить тексты ошибок SSO для администратора и пользователя

### Проблема

Часть ошибок сейчас сводится к общему `login failed`, что нормально для пользователя, но плохо для диагностики.

### Предложение

Разделить сообщения:

- user-safe generic message во frontend
- детализированная причина в admin diagnostics / audit / trace

Типовые причины:

- provider not initialized
- invalid state
- invalid nonce
- registration disabled
- email domain not allowed
- user locked

### Ожидаемый эффект

- лучше UX
- быстрее разбор проблем

## 17. Добавить SCIM capability section в основную документацию

### Проблема

SCIM уже реализован как существенная enterprise-функция, но он пока выглядит как техническая реализация, а не как полноценно оформленный продуктовый capability.

### Предложение

Добавить в основную docs-разметку:

- как включить SCIM
- какие endpoints поддерживаются
- как работает bearer token
- что такое `delete_action`
- примеры запросов
- ограничения текущей реализации

### Ожидаемый эффект

- легче внедрять SCIM
- меньше необходимости читать код

## Приоритет 5. Долгосрочные улучшения

## 18. Поддержать SCIM Groups

### Почему это полезно

Если продукт движется в сторону enterprise IAM, следующим логичным шагом станет:

- поддержка групп

Это может пригодиться для:

- массового назначения ролей
- управления доступом к интерфейсам
- синхронизации организационной структуры

### Что можно сделать поэтапно

1. read-only groups
2. group-to-role mapping
3. group-to-interface access mapping

## 19. Свести SSO, LDAP и SCIM в единый identity lifecycle layer

### Проблема

Сейчас есть три смежных механизма:

- SSO login
- LDAP sync
- SCIM provisioning

Они решают похожие задачи, но реализованы раздельно.

### Предложение

В долгую имеет смысл собрать общий lifecycle-слой:

- create / update / disable / delete policy engine
- источник изменения
- приоритет источников
- правила merge
- conflict resolution

### Ожидаемый эффект

- меньше расхождений между разными identity-каналами
- проще расширять продукт дальше

## 20. Ввести явную source precedence policy

### Проблема

Сейчас данные пользователя могут приходить из:

- локальной БД
- LDAP
- SSO
- SCIM

Но общая политика приоритета источников не выражена как продуктовая сущность.

### Предложение

Добавить configurable precedence, например:

- `local`
- `ldap`
- `sso`
- `scim`

Для полей:

- email
- first/last name
- department
- admin
- disabled state

### Ожидаемый эффект

- меньше конфликтов между интеграциями
- более предсказуемое поведение в enterprise окружениях

## Рекомендуемый порядок внедрения

## Быстрые победы

1. запретить изменение `userName` через SCIM без явной политики
2. валидировать `external_url` и callback-настройки на старте
3. добавить SCIM audit events
4. улучшить сообщения и diagnostics по SSO
5. добавить warning по использованию plain OAuth вместо OIDC

## Среднесрочные улучшения

1. health/status модель auth providers
2. preflight check для SSO-конфигурации
3. SCIM DB-side filtering для простых запросов
4. multi-token rotation для SCIM
5. email normalization и optional `email_verified`

## Долгосрочные улучшения

1. identity normalization layer
2. unified identity lifecycle layer
3. source precedence policy
4. SCIM Groups

## Итог

По результатам анализа видно, что текущая реализация уже сильная и практически полезная, особенно для self-hosted и enterprise-подобных сценариев. Основные точки роста лежат не в “добавить базовую функциональность”, а в следующем:

- сделать поведение более безопасным по умолчанию
- упростить диагностику интеграций
- повысить предсказуемость identity lifecycle
- подготовить SSO и SCIM к большим и более сложным инсталляциям

Если выбирать только три самые ценные оптимизации на ближайшую итерацию, я бы рекомендовал:

1. запрет / контроль переименования `userName` через SCIM
2. startup/preflight validation для SSO-конфигурации
3. полноценный audit и diagnostics слой для SCIM и внешних auth providers
