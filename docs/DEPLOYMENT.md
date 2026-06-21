# Self-Hosted Deployment

Stint is provider-neutral. Run the API container, the same image as a worker, the Next.js web container, PostgreSQL, and Redis on your own container platform.

## Services

- `api`: Go/Echo API on port `8080`
- `worker`: same image as `api`, command `worker`
- `web`: Next.js app on port `3000`
- `postgres`: PostgreSQL 15+
- `redis`: Redis 7+ for queues, rate limiting, and short-lived caches

## Required Environment

API and worker:

```bash
PORT=8080
BASE_URL=https://api.example.com
WEB_BASE_URL=https://app.example.com
DATABASE_URL=postgres://USER:PASSWORD@HOST:5432/stint?sslmode=require
REDIS_URL=redis://HOST:6379
GITHUB_CLIENT_ID=...
GITHUB_CLIENT_SECRET=...
SESSION_SECRET=generate-a-long-random-secret
ENABLE_PUBLIC_LEADERBOARD=true
ENABLE_REGISTRATION=true
MAX_USERS=0
DEV_SEED_ENABLED=false
HEARTBEAT_RETENTION_DAYS=365
```

`BASE_URL` is the public API origin used for GitHub OAuth callbacks and WakaTime-compatible API URLs. `WEB_BASE_URL` is the browser app origin used after login and for web CORS.

For public `BASE_URL` values, startup validates `SESSION_SECRET` and rejects missing, short, or placeholder values such as `change-me-in-production`. Generate at least 32 random bytes, for example:

```bash
openssl rand -hex 32
```

Public `BASE_URL` deployments also validate that `GITHUB_CLIENT_ID` and `GITHUB_CLIENT_SECRET` are set so GitHub-only SSO works at first boot. The only exception is an explicitly private test environment with `DEV_SEED_ENABLED=true`.

Set `ENABLE_REGISTRATION=false` after your own account exists to close GitHub signups while keeping existing users able to log in. Set `MAX_USERS` to a positive integer to cap total local accounts. Set `ENABLE_PUBLIC_LEADERBOARD=false` to disable `/api/v1/leaders`; private leaderboards and tokenized share links are unaffected.

Set `HEARTBEAT_RETENTION_DAYS` above `0` to apply one global heartbeat retention window for all users. Set it to `0` to let each user control retention from Settings with `heartbeat_retention_days`; a per-user value of `0` keeps all heartbeats.

Data dumps currently support local JSON snapshots and require `STORAGE_TYPE=local`. Startup rejects other storage types so deployments do not silently fall back to the wrong backend. The API or worker writes completed `heartbeats` and `daily` dump files under `STORAGE_PATH`, records the authenticated download URL, and serves the stored snapshot as a raw top-level JSON array. The app also loads the remote storage/email settings from the spec so you can keep one environment file as S3 and notification support are added:

```bash
STORAGE_TYPE=local
STORAGE_PATH=./data/dumps
S3_BUCKET=
S3_REGION=us-east-1
AWS_ACCESS_KEY_ID=
AWS_SECRET_ACCESS_KEY=
SMTP_HOST=
SMTP_PORT=587
SMTP_USER=
SMTP_PASS=
EMAIL_FROM=noreply@example.com
```

When API and worker run as separate containers, mount the same persistent volume at `STORAGE_PATH` for both services. The included Docker image creates `/data/dumps` for the non-root runtime user, and the Compose file mounts a shared `dumpdata` volume there.

Web:

```bash
API_BASE_URL=https://api.example.com
NEXT_PUBLIC_API_BASE_URL=
```

Set `NEXT_PUBLIC_API_BASE_URL` only if browser-side requests need a different public API origin. The default same-origin/proxy behavior is suitable when the web app forwards API calls server-side.

## Build Images

```bash
docker build -t stint-api .
docker build -t stint-web ./web
```

Run the worker from the API image:

```bash
docker run stint-api worker
```

## Reverse Proxy

Expose:

- `https://api.example.com` to the API container on `:8080`
- `https://app.example.com` to the web container on `:3000`

Preserve `Authorization`, cookies, request bodies, and standard forwarded headers. Heartbeat clients send small JSON payloads, but data-dump imports can be larger.

The local Compose file includes an optional Caddy profile:

```bash
docker compose --profile proxy up -d caddy
```

By default it exposes API `http://localhost:8081` and web `http://localhost:3002`. For a real host, set `STINT_API_SITE` and `STINT_WEB_SITE` to your public Caddy site addresses, map ports `80`/`443`, and keep `BASE_URL`/`WEB_BASE_URL` aligned with those public origins.

## Startup Order

1. Start PostgreSQL and Redis.
2. Start one API instance. It runs migrations under a database advisory lock.
3. Start one or more worker instances.
4. Start the web app.

Multiple API/worker instances are supported; migrations are serialized by the advisory lock.

## Operational Checks

```bash
curl -fsS https://api.example.com/healthz
curl -fsS https://api.example.com/api/v1/docs
curl -fsSI https://app.example.com/login
```

Run the compatibility smoke against a deployed API with:

```bash
API_BASE=https://api.example.com scripts/smoke-wakatime.sh
```

`DEV_SEED_ENABLED` defaults to `false` when `BASE_URL` is not localhost. Only enable `DEV_SEED_ENABLED=true` for local or private test environments.
