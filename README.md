# Платформа записи на приём

Backend-проект для онлайн-записи на приём с событийной архитектурой.

## Сервисы

### 1) booking-service (`cmd/app`)

Назначение:
- REST API для специалистов, клиентов, расписаний, слотов и записей.
- Аутентификация и роли (`admin`, `client`, `specialist`) через JWT.
- Гарантированная публикация событий в Kafka через паттерн Outbox.
- Генерация событий-напоминаний (24 часа и 1 час до записи).

Что делает:
- Создаёт/переносит/отменяет записи.
- Контролирует, чтобы у специалиста не было пересекающихся записей.
- Публикует события `appointment.*` для остальных сервисов.

### 2) notification-service (`cmd/notification`)

Назначение:
- Отправка уведомлений клиенту в Telegram на основе событий из Kafka.

Что делает:
- Читает события записи (`created`, `rescheduled`, `canceled`, напоминания).
- Проверяет идемпотентность по `event_key`.
- Отправляет сообщения через Telegram Bot API.
- Пишет статус обработки в БД (`sent`, `failed`, `skipped`).

### 3) calendar-sync-service (`cmd/calendar`)

Назначение:
- Синхронизация событий записи с Google Calendar.

Что делает:
- Читает события из Kafka.
- Для `created/rescheduled` создаёт или обновляет событие в Google Calendar.
- Для `canceled` удаляет событие из календаря.
- Хранит связь `appointment_id -> google_event_id` и журнал синхронизации.

### 4) billing-service (`cmd/billing`)

Назначение:
- Выставление и сопровождение счетов по записям.

Что делает:
- Читает события из Kafka и создаёт/обновляет инвойсы.
- При отмене записи обрабатывает отмену счёта или возврат (refund).
- Предоставляет HTTP API для просмотра и оплаты инвойса.

## Типичный flow

1. Клиент вызывает `booking-service` по REST.
2. `booking-service` пишет изменения в БД и событие в `outbox_events` в одной транзакции.
3. Outbox relay публикует событие в Kafka (`appointments.events.v1`).
4. `notification-service`, `calendar-sync-service`, `billing-service` читают событие как независимые consumers.
5. Каждый consumer обрабатывает событие идемпотентно и пишет свой результат в свою БД.

## Технологии

- Go 1.24+
- PostgreSQL
- Kafka (KRaft)
- Docker + Docker Compose
- `github.com/jackc/pgx/v5`
- `github.com/segmentio/kafka-go`
- `github.com/golang-jwt/jwt/v5`
- `golang.org/x/crypto/bcrypt`
- `slog` (structured logging)

## Структура проекта

```text
.
├── cmd
│   ├── app                # booking-service
│   ├── notification       # notification-service
│   ├── calendar           # calendar-sync-service
│   └── billing            # billing-service
├── internal
│   ├── booking
│   ├── notifier
│   ├── calendar
│   ├── billing
│   ├── config
│   └── ...
├── migrations
│   ├── booking
│   ├── notification
│   ├── calendar
│   └── billing
├── deployments
├── docker-compose.yml
└── README.md
```

Внутри сервисов используется onion architecture:
- `domain` — сущности и правила предметной области
- `application` — use-cases
- `infrastructure` — БД/Kafka/внешние API
- `transport` — HTTP/Kafka adapters
- `bootstrap` — сборка зависимостей и запуск

## Быстрый запуск (Docker Compose)

### 1) Подготовка env

```bash
cp .env.example .env
```

При необходимости настройте в `.env`:
- Telegram: `TELEGRAM_BOT_TOKEN`
- Google Calendar: `GOOGLE_CALENDAR_ID` и токен/сервисный аккаунт
- JWT: `BOOKING_JWT_SECRET`, `BOOKING_JWT_TTL_MINUTES`

### 2) Запуск всех сервисов

```bash
docker compose up --build
```

Что поднимется:
- PostgreSQL
- Kafka
- миграции всех БД
- booking-service
- notification-service
- calendar-sync-service
- billing-service

### 3) Проверка

- Booking API: `http://localhost:8080`
- Billing API: `http://localhost:8081`
- Healthcheck: `GET http://localhost:8080/healthz`

### 4) Остановка

```bash
docker compose down
```

С удалением тома БД:

```bash
docker compose down -v
```

## Основные API

### booking-service

Публичные endpoints:
- `GET /healthz`
- `POST /auth/register`
- `POST /auth/login`

Защищённые endpoints (`Authorization: Bearer <token>`):
- `GET /specialists`
- `POST /specialists`
- `POST /clients`
- `POST /specialists/{id}/schedule`
- `PUT /specialists/{id}/schedule`
- `GET /specialists/{id}/schedule?date=YYYY-MM-DD`
- `GET /specialists/{id}/slots?date=YYYY-MM-DD`
- `POST /appointments`
- `PATCH /appointments/{id}/reschedule`
- `PATCH /appointments/{id}/cancel`

### billing-service

- `GET /billing/invoices/{appointment_id}`
- `POST /billing/invoices/{appointment_id}/pay`

## Роли и доступ

- `admin`: полный доступ к операциям booking-service.
- `client`: доступ к своим записям и общим операциям просмотра.
- `specialist`: доступ к своему расписанию и своим записям.

## Базы данных

- `booking_db`: специалисты, клиенты, расписания, записи, outbox, пользователи.
- `notification_db`: обработанные события и логи отправок.
- `calendar_db`: обработанные события, маппинг календарных событий, логи синхронизации.
- `billing_db`: обработанные события, инвойсы, биллинговые логи.

## Локальная сборка

```bash
go build ./cmd/app ./cmd/notification ./cmd/calendar ./cmd/billing
go test ./...
```
