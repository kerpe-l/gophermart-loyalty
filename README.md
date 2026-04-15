# Gophermart — система лояльности

HTTP API накопительной системы лояльности: регистрация пользователей, приём номеров заказов, начисление баллов через внешний accrual-сервис, списание баллов.

Дипломный проект курса «Go-разработчик» Яндекс Практикума.

## Стек

- Go 1.25
- PostgreSQL (через `pgx/v5`)
- `go-chi/chi` — роутер
- `go.uber.org/zap` — логирование
- `golang-jwt/jwt` — аутентификация
- `bcrypt` — хеширование паролей

## Запуск

```bash
go run ./cmd/gophermart \
  -a localhost:8080 \
  -d postgres://user:pass@localhost:5432/gophermart \
  -r http://localhost:8081
```

### Параметры

| Флаг | Переменная окружения | Описание |
|------|----------------------|----------|
| `-a` | `RUN_ADDRESS` | адрес и порт HTTP-сервера (по умолчанию `localhost:8080`) |
| `-d` | `DATABASE_URI` | строка подключения к PostgreSQL |
| `-r` | `ACCRUAL_SYSTEM_ADDRESS` | адрес accrual-сервиса |

Переменные окружения имеют приоритет над флагами. Миграции накатываются автоматически при старте.

Accrual-сервис для локальной разработки лежит в [cmd/accrual](cmd/accrual) (бинарники под Linux/macOS/Windows).

## Эндпойнты

Публичные:
- `POST /api/user/register` — регистрация
- `POST /api/user/login` — аутентификация, возвращает JWT в заголовке `Authorization`

Защищённые (требуют `Authorization: Bearer <token>`):
- `POST /api/user/orders` — загрузка номера заказа (валидация Luhn)
- `GET  /api/user/orders` — список заказов пользователя
- `GET  /api/user/balance` — текущий баланс и сумма списаний
- `POST /api/user/balance/withdraw` — списание баллов
- `GET  /api/user/withdrawals` — история списаний

Полная спецификация — [SPECIFICATION.md](SPECIFICATION.md).

## Структура

```
cmd/gophermart       — точка входа
cmd/accrual          — бинарники accrual-сервиса для локальной разработки
internal/config      — флаги и env
internal/handler     — HTTP-хендлеры
internal/middleware  — auth, gzip, logging
internal/auth        — JWT, bcrypt
internal/accrual     — клиент и поллер accrual-сервиса
internal/storage/postgres — репозитории и SQL-миграции
internal/model       — доменные типы
internal/apperrors   — sentinel-ошибки домена
internal/luhn        — валидация номеров заказов
internal/logger      — обёртка zap
```

## Тесты

```bash
go test ./... -race
```

## Обновление шаблона

Чтобы получать обновления автотестов и других частей шаблона, выполните команду:

```
git remote add -m master template https://github.com/yandex-praktikum/go-musthave-diploma-tpl.git
```

Для обновления кода автотестов выполните команду:

```
git fetch template && git checkout template/master .github
```

Затем добавьте полученные изменения в свой репозиторий.
