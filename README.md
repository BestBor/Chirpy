# Chirpy API

A Twitter-like microblogging REST API built in Go. Users can post short messages ("chirps"), manage accounts, and subscribe to **Chirpy Red** — a premium tier unlocked via webhook.

---

## Tech Stack

- **Language:** Go
- **Database:** PostgreSQL (via `sqlc` generated queries)
- **Auth:** JWT (access tokens) + opaque refresh tokens
- **Router:** `net/http` (stdlib)
- **Deps:** `godotenv`, `google/uuid`, `lib/pq`

---

## Environment Variables

Create a `.env` file in the project root:

```env
DB_URL=postgres://user:password@localhost:5432/chirpy?sslmode=disable
PLATFORM=dev         # set to "dev" to enable /admin/reset
SECRET=your_jwt_secret
POLKA_KEY=your_polka_webhook_key
```

---

## Running the Server

```bash
go run . 
```

Server starts on **port 8080**.

---

## API Reference

### Health

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/healthz` | Returns `200 OK` if server is up |

---

### Users

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `POST` | `/api/users` | None | Create a new user |
| `PUT` | `/api/users` | JWT | Update email and password |

#### POST `/api/users`

**Request body:**
```json
{
  "email": "user@example.com",
  "password": "secret"
}
```

**Response `201`:**
```json
{
  "id": "uuid",
  "created_at": "...",
  "updated_at": "...",
  "email": "user@example.com",
  "is_chirpy_red": false
}
```

#### PUT `/api/users`

Requires `Authorization: Bearer <access_token>`.

**Request body:**
```json
{
  "email": "new@example.com",
  "password": "newpassword"
}
```

---

### Authentication

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/login` | Log in, receive JWT + refresh token |
| `POST` | `/api/refresh` | Exchange refresh token for new JWT |
| `POST` | `/api/revoke` | Revoke a refresh token |

#### POST `/api/login`

**Request body:**
```json
{
  "email": "user@example.com",
  "password": "secret"
}
```

**Response `200`:**
```json
{
  "id": "uuid",
  "email": "user@example.com",
  "token": "",
  "refresh_token": "",
  "is_chirpy_red": false
}
```

#### POST `/api/refresh`

Requires `Authorization: Bearer <refresh_token>`.

**Response `200`:**
```json
{
  "token": ""
}
```

#### POST `/api/revoke`

Requires `Authorization: Bearer <refresh_token>`. Returns `204 No Content`.

> **Token lifetimes:** JWTs expire after **1 hour**. Refresh tokens expire after **60 days**.

---

### Chirps

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/chirps` | None | Get all chirps (sorted by `created_at`) |
| `GET` | `/api/chirps?author_id={uuid}` | None | Get chirps by a specific user |
| `GET` | `/api/chirps/{chirpID}` | None | Get a single chirp by ID |
| `POST` | `/api/chirps` | JWT | Create a new chirp |
| `DELETE` | `/api/chirps/{chirpID}` | JWT | Delete a chirp (owner only) |

#### POST `/api/chirps`

Requires `Authorization: Bearer <access_token>`.

**Request body:**
```json
{
  "body": "Hello, world!"
}
```

**Response `201`:**
```json
{
  "id": "uuid",
  "created_at": "...",
  "updated_at": "...",
  "body": "Hello, world!",
  "user_id": "uuid"
}
```

**Constraints:**
- Max 140 characters
- Profanity filter: `kerfuffle`, `sharbert`, `fornax` → `****`

#### DELETE `/api/chirps/{chirpID}`

Returns `204 No Content` on success. Returns `403 Forbidden` if the authenticated user doesn't own the chirp.

---

### Chirpy Red (Webhooks)

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `POST` | `/api/polka/webhooks` | API Key | Upgrade a user to Chirpy Red |

Requires `Authorization: ApiKey <polka_key>`.

**Request body:**
```json
{
  "event": "user.upgraded",
  "data": {
    "user_id": "uuid"
  }
}
```

Returns `204 No Content`. Events other than `user.upgraded` are silently ignored.

---

### Admin

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/admin/metrics` | HTML page showing file server hit count |
| `POST` | `/admin/reset` | Reset hit counter and wipe all users/chirps (dev only) |

> `/admin/reset` only works when `PLATFORM=dev`.

---

### Static Files

The server also serves static files from the project root under `/app/`. Each request increments the hit counter tracked by `/admin/metrics`.

---

## Error Responses

All errors return JSON in this shape:

```json
{
  "error": "description of what went wrong"
}
```

Common status codes:

| Code | Meaning |
|------|---------|
| `400` | Bad request / invalid input |
| `401` | Missing or invalid credentials |
| `403` | Forbidden (authenticated but not authorized) |
| `404` | Resource not found |
| `500` | Internal server error |