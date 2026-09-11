# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Feature flag proxy server for BetterCity. Sits between Flutter mobile apps and a GO Feature Flag (GOFF) relay proxy, providing bulk flag evaluation with Keycloak authentication and device-context targeting. Backend services consume the GOFF relay directly via SDK — this API serves only the Flutter app.

**Stack**: Go 1.23+, Echo v4, Uber FX (DI), OpenFeature SDK, GO Feature Flag provider, Keycloak (gocloak), Cobra/Viper, testify.

## Build & Run Commands

```bash
# Start relay proxy + API server (Docker)
make up

# Run API server locally (connects to relay on localhost:1031)
make run                  # sets APP_ENV=local automatically

# Run tests
make test                 # or: go test ./... -race -v

# Build binary
go build -o bin/server .

# Stop containers
make down

# Tear down and clean
make clean
```

## Architecture

**Request flow**: `main.go` → Cobra CLI (`cmd/`) → Uber FX boots modules → Echo HTTP server starts.

**Interface-based design**: All services expose interfaces (`FeatureFlagEvaluator`, `TokenValidator`, `FlagRegistry` in `internal/services/interfaces.go`). Handlers and middleware depend on interfaces, not concrete types. FX provides concrete implementations via `fx.As` annotations.

**FX module loading order** (in `cmd/server.go`):
1. `config.Module` — Viper loads `config/{APP_ENV}.yaml`, merges env vars, loads `.env` for local dev
2. `services.Module` — FeatureFlagService, KeycloakService, FlagRegistryService (all as interfaces)
3. `handlers.Module` — FlagsHandler, HealthHandler
4. `middleware.Module` — OptionalJWT auth, CORS (configurable), request logger, request ID, rate limiter
5. `fxserver.Module` (`internal/fx/`) — Echo instance, route registration, lifecycle hooks

**Key packages**:
- `internal/services/registry.go` — Loads `flags/apps/<app>.yaml` (the same GOFF files the relay serves) at startup for each app in `ServedApps`. Type is inferred from `variations`, fallback default from `defaultRule.variation`. Adding a flag = editing the GOFF YAML only.
- `internal/services/interfaces.go` — All service interfaces. Handlers depend on these, not concrete types.
- `internal/middleware/auth.go` — OptionalJWT: validates via Keycloak introspection if Bearer token present, otherwise falls back to device headers.
- `internal/middleware/requestid.go` — Generates or propagates `X-Request-ID` for log correlation.
- `internal/middleware/ratelimit.go` — Token-bucket rate limiter per IP, configurable via `app.rate_limit`.

**Endpoints**:
- `GET /health` — liveness probe (always 200)
- `GET /ready` — readiness probe (checks GOFF relay connectivity via flag evaluation)
- `GET /api/v1/flags?app=flutter` — bulk flag evaluation; `app` param defaults to `flutter`; auth optional

## Configuration

Config loaded via Viper from `config/{APP_ENV}.yaml` (APP_ENV defaults to "local"). Environment variables override YAML using underscore-separated paths (e.g., `KEYCLOAK_CLIENT_SECRET` → `keycloak.client_secret`). A `.env` file is loaded automatically for local development.

**Key config fields** in `app`:
- `log_level` — debug/info/warn/error (applied to slog)
- `cors_origins` — list of allowed origins (default: `["*"]`)
- `rate_limit` — requests per second per IP (default: 100)

## Docker

Two images built in CI (`ci.yml`, gated by test job):
- **Dockerfile** — GOFF relay proxy, serves flag files from `flags/`
- **Dockerfile.api** — Multi-stage Go build for the API server

`docker-compose.yml` runs both relay + API locally. Network `bettercity_local` is created automatically.

## Flag Definitions

**GOFF relay flags** (targeting rules, loaded by relay):
- `flags/apps/flutter.yaml` — Flutter app flags
- `flags/shared.yaml` — Cross-application flags (consumed by backends via GOFF SDK)

**API flag registry**: there is no separate registry file. The API reads `flags/apps/<app>.yaml` directly for the apps listed in `ServedApps` (`internal/services/registry.go`, currently only `flutter`). Every flag must have homogeneous scalar `variations` (bool/string/int/float) and a `defaultRule.variation`; otherwise the API refuses to start. `flags/shared.yaml` is relay-only. All flag names use **snake_case**. See `docs/adr/0001-goff-yaml-fonte-unica.md`.

## Agent skills

### Issue tracker

Issues and specs live as local markdown files under `.scratch/<feature>/`. See `docs/agents/issue-tracker.md`.

### Triage labels

Default vocabulary: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `CONTEXT.md` at the repo root plus `docs/adr/`. See `docs/agents/domain.md`.
