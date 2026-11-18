# URL Shortener на Go

HTTP-сервис сокращения ссылок. Реализован на стандартной библиотеке Go 1.23, без БД и сторонних зависимостей.

## Запуск

Из корня проекта:

```bash
go run ./cmd/shortener
```

Сервис слушает `http://localhost:8080`. Проверка:

```bash
go test ./...
go test -race ./...
```

Дополнительная настройка (не обязательна для проверки): переменная `SHORTENER_SECRET` задаёт секрет для HMAC-SHA256; `PORT` задаёт порт прослушивания (по умолчанию `8080`); `PUBLIC_BASE_URL` задаёт базовый адрес выдаваемых ссылок (по умолчанию `http://localhost:<PORT>`). Без переменной `SHORTENER_SECRET` используется **только учебный** ключ. Для реального развертывания нужен непубличный секрет и HTTPS с флагом `Secure` у cookie.

## API и примеры

Примеры ниже выполняются из bash; для PowerShell используйте `curl.exe`.

### POST /api/shorten

```bash
curl -i -c cookies.txt -b cookies.txt \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://example.com/some/long/path"}' \
  http://localhost:8080/api/shorten
```

Успешный ответ `200 OK`:

```json
{"result":"http://localhost:8080/<short-id>"}
```

Новый пользователь получает `Set-Cookie: user_id=<ID>:<HMAC_SHA256>`. Некорректный JSON или URL дают `400 Bad Request`.

### GET /{id}

```bash
curl -i http://localhost:8080/<short-id>
```

При наличии ссылки возвращается `307 Temporary Redirect` с заголовком `Location: <original_url>`; неизвестный ID — `404 Not Found`.

### GET /api/user/urls

```bash
curl -i -b cookies.txt http://localhost:8080/api/user/urls
```

Ответ `200 OK` для пользователя со ссылками:

```json
[{"short_url":"http://localhost:8080/<short-id>","original_url":"https://example.com/some/long/path"}]
```

Если ссылок нет, ответ `204 No Content`. Если cookie `user_id` присутствует, но её значение пустое или формат нарушен — `401 Unauthorized`. Отсутствующая cookie либо корректная по формату cookie с неправильной HMAC-подписью приводят к выдаче новой cookie и `204 No Content` (для нового пользователя).

## Структура

```text
shortener/
├── cmd/shortener/main.go                   # точка входа, конфигурация и HTTP-сервер
├── internal/app/app.go                     # маршрутизация, обработчики, валидация
├── internal/app/app_test.go                # HTTP-тесты через httptest
├── internal/auth/cookie.go                 # генерация ID, HMAC-SHA256, cookie
├── internal/auth/cookie_test.go
├── internal/storage/memory.go              # потокобезопасное in-memory хранилище
├── internal/storage/memory_test.go
├── go.mod
└── README.md
```

Хранилище содержит `originalByID` (`id -> URL`) и `urlsByUser` (`userID -> []UserURL`). Короткие ID генерируются криптографически случайно, коллизии проверяются при записи. Данные пропадают после остановки процесса: это сознательное ограничение задания.
