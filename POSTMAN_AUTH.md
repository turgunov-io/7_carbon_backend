# Авторизация и Postman

## Что изменено
- Все эндпоинты `"/admin/*"` и `"/admin/storage/*"` теперь требуют авторизацию.
- Добавлен вход: `POST /admin/auth/login`.
- Добавлена проверка текущей сессии: `GET /admin/auth/me`.

## Варианты авторизации

### 1) Рекомендуемый (логин/пароль + bearer токен)
Нужно задать переменные окружения:

```env
ADMIN_USERNAME=admin
ADMIN_PASSWORD=change_me_strong_password
JWT_SECRET=change_me_strong_secret
# опционально (по умолчанию 12h):
ADMIN_SESSION_TTL=12h
```

`JWT_SECRET` можно заменить на `ADMIN_JWT_SECRET`.

### 2) Совместимость со старой схемой (статический токен)

```env
ADMIN_TOKEN=change_me_admin_token
```

Можно передавать:
- `Authorization: Bearer <ADMIN_TOKEN>`
- или `X-Admin-Token: <ADMIN_TOKEN>`

## Настройка Postman Environment
Создайте environment c переменными:
- `base_url` = `http://localhost:7777` (или ваш адрес)
- `admin_username`
- `admin_password`
- `access_token` (пусто на старте)
- `admin_token` (если используете статический токен)

## Запросы для Postman

### 1) Логин
`POST {{base_url}}/admin/auth/login`

Headers:
- `Content-Type: application/json`

Body (raw JSON):

```json
{
  "username": "{{admin_username}}",
  "password": "{{admin_password}}"
}
```

Успех `200 OK`, пример ответа:

```json
{
  "status": "success",
  "data": {
    "token_type": "Bearer",
    "access_token": "<token>",
    "expires_at": "2026-02-27T08:00:00Z",
    "expires_in": 43200,
    "username": "admin"
  }
}
```

Tests script (сохранить токен):

```javascript
const json = pm.response.json();
pm.environment.set("access_token", json.data.access_token);
```

### 2) Проверка токена
`GET {{base_url}}/admin/auth/me`

Headers:
- `Authorization: Bearer {{access_token}}`

Успех `200 OK`:

```json
{
  "status": "success",
  "data": {
    "authenticated": true,
    "auth_type": "bearer",
    "username": "admin",
    "expires_at": "2026-02-27T08:00:00Z"
  }
}
```

### 3) Любой защищенный admin endpoint
Пример: `GET {{base_url}}/admin/banners`

Headers:
- `Authorization: Bearer {{access_token}}`

Или (для старого режима):
- `X-Admin-Token: {{admin_token}}`

## Частые ошибки
- `401 missing admin token`: не передан токен.
- `401 unauthorized`: неверный/просроченный токен.
- `500 admin auth is not configured`: не задано ни `ADMIN_TOKEN`, ни пара `ADMIN_USERNAME` + `ADMIN_PASSWORD`.
- `503 admin login is not configured`: вызван `/admin/auth/login`, но не настроены `ADMIN_USERNAME` и `ADMIN_PASSWORD`.
