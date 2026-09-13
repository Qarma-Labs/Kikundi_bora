# PR: Docker ports → 5050 (backend) / 5051 (frontend) + one-command setup

## What existed BEFORE this change (investigated, not assumed)

- **`setup.sh` already existed** and was already a solid Docker-based one-command
  script: prerequisite checks, random-secret `.env` generation, free-port picking
  for Postgres, `docker compose build/up`, DB + backend health wait loops,
  idempotent `-migrate` step, demo-login verification, and a final banner.
  This PR **extends it** (no duplicate created): new port defaults and a
  `docker compose` v2 plugin check.
- **`start.sh` existed** — that is the *bare-metal* dev launcher (runs the Go
  binary + Vite directly, no Docker). It runs the app on its **internal** listen
  ports (8080/8081), which by design do NOT change — so `start.sh` is untouched.
- **`backend/.env.example` existed** — kept as the template; only its port
  references were updated.
- **No stop script existed** → `stop.sh` is new.
- **Port mapping (verified, not assumed):**
  | Service | Container internal | Old host port | New host port |
  |---|---|---|---|
  | Backend Go API (`backend/Dockerfile`, distroless) | **8080** | 8080 | **5050** |
  | Frontend nginx SPA (`Frontend-1/Dockerfile` + `nginx.conf` listen 8080) | **8080** | 8081 | **5051** |

  → **Internal ports stay 8080**; only host-side mappings changed. The mapping
  semantics are preserved exactly: 5050=backend (same slot as old 8080),
  5051=frontend (same slot as old 8081).

## Part 1 — port change (5050 / 5051)

| File | Change |
|---|---|
| `docker-compose.yml` | `${BACKEND_PORT:-5050}:8080`, `${FRONTEND_PORT:-5051}:8080`; `PUBLIC_BASE_URL` + `VITE_API_URL` defaults → 5050 |
| `docker-compose.prod.yml` | defaults → 5050/5051 — **the 127.0.0.1 bindings are preserved** (backend stays non-public; the host Nginx remains the only entry point) |
| `backend/.env.example` | `PUBLIC_BASE_URL=http://localhost:5050`, CORS includes 5051; `PORT=8080` documented as the *internal* port |
| `backend/Dockerfile` | `PUBLIC_BASE_URL` default → 5050 (`EXPOSE 8080` unchanged — internal) |
| `Frontend-1/Dockerfile` | `ARG VITE_API_URL` default → `http://localhost:5050/api/v1` (deploy workflow still overrides with the public https URL) |
| `.github/workflows/deploy.yml` | port-conflict guard + health checks 8080/8081 → **5050/5051**; deploy prints a Nginx reminder |
| `setup.sh` | defaults + generated env + final banner → 5050/5051; added `docker compose` plugin check |
| `README.md` | Docker Quick Start documents 5050/5051 + `./stop.sh`; bare-metal section marked as internal-port dev |
| `docs/deploy/nginx-site.conf` | **NEW** — host Nginx template for `kikundi-test.ditronics.co.tz` pointing at 127.0.0.1:5051 (SPA) and 127.0.0.1:5050 (`/api/*`) |

### Host Nginx (critical manual step on the VPS)

The reverse proxy for `kikundi-test.ditronics.co.tz` lives at host level — it is
not in the repo and cannot be updated from here. `docs/deploy/nginx-site.conf`
is the updated template: proxy_pass targets must change **8081 → 5051** (SPA)
and **8080 → 5050** (`/api/*`), applied on the VPS together with (or before)
pulling this change, or the subdomain breaks. The deploy workflow now prints
this reminder after every deploy.

### Deliberately unchanged

- **Internal container port 8080** on both images (compose maps host→internal).
- **Bare-metal dev** (`start.sh`, `go run .`, `npm run dev`) keeps 8080/8081 —
  those are the app's internal ports, per the task's keep-internal guidance.
- Historical pentest/audit docs under `docs/` reference the ports used at the
  time of testing — left as historical record.

## Part 2 — one-command setup

- `setup.sh` (extended): prerequisite check now also verifies the
  `docker compose` v2 plugin with a clear install pointer; ports default to
  5050/5051; everything else (idempotent `.env` generation, build, up,
  DB health wait, idempotent migrate+seed, verification, final banner) already
  existed and is kept. Safe to re-run — existing `.env`, data and running
  containers are never clobbered (`--reset-db` is the explicit destructive path).
- `stop.sh` (new): `docker compose down` by default (data kept), `-v` to also
  wipe. Host Nginx is not containerized and is intentionally untouched.

## Verification (fresh clone, zero manual steps)

- Fresh clone into a clean directory (`git clone … /tmp/…`), ports 5050/5051
  verified free, then `./setup.sh` — **full pass, exit 0**: images built,
  containers up, PostgreSQL healthy, migrations + seed applied, `/health` 200
  on :5050, demo login works (`asha@kikundi.tz`), SPA 200 on :5051, and the
  SPA's `/api/*` proxy reaches the backend (login through :5051 returns a JWT).
- CORS verified: `Access-Control-Allow-Origin: http://localhost:5051` on the
  backend response for the SPA origin.
- **Idempotency:** re-running `./setup.sh` on the already-running stack exits
  green ("backend/.env already exists — keeping it"), keeps data, re-verifies.
- **`./stop.sh` verified**: containers down, data volume kept; `docker compose
  up -d` brings it back healthy on the same ports.
- Two real bugs were found by the fresh-clone test and fixed in this PR:
  1. `stop.sh` initially failed because compose interpolation requires
     `POSTGRES_PASSWORD` even for `down` — now wired from `backend/.env`.
  2. `setup.sh` could generate a brand-new DB password when an existing
     `kikundi-bora_pgdata` volume had been initialized with a different one
     (SASL 28P01 auth failure with no hint) — it now **recovers** the existing
     database password from the previous `kikundi-db` container before
     generating `.env`.

## Post-change smoke checks

- Backend `/health` responds on `:5050`; SPA responds on `:5051`.
- Frontend → backend connectivity: the SPA container proxies `/api/*` to
  `http://backend:8080` over the docker network (unchanged), and the
  browser-facing `VITE_API_URL` now points at `http://localhost:5050/api/v1`.
- Public subdomain: works once the host Nginx proxy_pass targets are updated
  per `docs/deploy/nginx-site.conf` (template provided).
