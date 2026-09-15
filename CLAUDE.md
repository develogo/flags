# CLAUDE.md

Guidance for Claude Code when working in this repository.

## Project Overview

Feature flag proxy for BetterCity. Sits between the Flutter mobile app and a GO Feature Flag (GOFF) relay proxy: bulk flag evaluation, targeting from client-declared headers. Backend services consume the relay directly via SDK — this API serves only the Flutter app.

**Stack**: Go 1.23+, Echo v4, Uber FX (DI), OpenFeature SDK + GOFF provider, Cobra/Viper, testify.

## Build & Run

`make help` lists the targets. What the Makefile does not tell you:

- `make run` sets `APP_ENV=local` and expects the relay on `localhost:1031`. Run it from the repo root: `flags/apps` is resolved relative to the cwd (there is no config key for it).
- `make up` requires the external Docker network `bettercity_local` to already exist (`docker network create bettercity_local`); compose does not create it. Relay on `:1031`, API on `:1324`.
- `make test` runs `go test ./... -race -v`. Registry fixtures (valid and invalid GOFF files) live in `testdata/`.

## Architecture

**Request flow**: `main.go` → Cobra (`cmd/`) → Uber FX modules in the order listed in `cmd/server.go` (`config` → `services` → `handlers` → `middleware` → `internal/fx` server) → Echo.

**Interface-based design**: every service is exposed as an interface in `internal/services/interfaces.go` (`FeatureFlagEvaluator`, `FlagRegistry`). Handlers depend on those; FX binds the concrete types via `fx.As`. Add new services the same way.

**Routes** (`internal/fx/fx.go`):
- `GET /health` — liveness, always 200
- `GET /ready` — readiness; evaluates a flag against the relay
- `GET /api/v1/flags?app=<app>` — bulk evaluation; missing `app` or `app=flutter` resolves to `bettercity-flutter` (legacy alias, handler only)

Only the `/api/v1` group gets the per-IP rate limiter (`app.rate_limit`) and `ClientContext` (`internal/middleware/clientcontext.go`). It never rejects a request and has no auth: it reads the device headers (`Device-ID`, `Platform`, `App-Version`, …) plus optional `User-ID`; `Authorization` is ignored. Targeting key is `User-ID`, else `Device-ID`. `X-Request-ID` is generated or propagated on every request for log correlation.

## Configuration

Viper loads `config/{APP_ENV}.yaml` (`APP_ENV` defaults to `local`); env vars override using underscore paths (`GOFF_ENDPOINT` → `goff.endpoint`). A `.env` in the cwd is read for local secrets, but already-set env vars win over it. Fields, defaults and validation: `internal/config/config.go`.

## Flag Definitions

The GOFF YAML files under `flags/apps/` are the single source of truth — see `docs/adr/0001-goff-yaml-fonte-unica.md` and `docs/adr/0002-servico-multi-app-flag-set-por-app.md`. One file per app, the file name is the app name. The relay serves each app as a flag set whose API key is the app name; the API reads only the apps listed in `ServedApps` (`internal/services/registry.go`, currently `bettercity-flutter`) and evaluates them with one OpenFeature client per app:

- `flags/apps/bettercity-flutter.yaml` — served by relay and API
- `flags/apps/bettercity-api.yaml` — relay-only, consumed by the backend via SDK with `APIKey: "bettercity-api"`. Keep it out of `ServedApps`: the API endpoint is unauthenticated.

Invariants the API enforces at startup (it refuses to boot otherwise): homogeneous scalar `variations` (bool/string/int/float) and a `defaultRule.variation` naming one of them. Percentage rollouts live in `targeting` rules, so `defaultRule` always resolves to a single fallback value. Flag names are **snake_case**. Adding a flag = editing the YAML; serving a new app = one entry in `ServedApps` plus `flags/apps/<app>.yaml`.

## Docker & CI

- `Dockerfile` — relay image: a Go stage runs `relay-config` to generate the relay config (one flag set per app); the pinned GOFF image gets `flags/` + the generated config. There is no versioned relay config file
- `Dockerfile.api` — multi-stage API build; copies `config/` and `flags/` into the image

`ci.yml` builds and pushes both images after the test job and opens a PR in `develogo/stacks`. Deploy details: `DEPLOYMENT.md`.

## Agent skills

### Issue tracker

Issues and specs live as local markdown files under `.scratch/<feature>/`. See `docs/agents/issue-tracker.md`.

### Triage labels

Default vocabulary: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `CONTEXT.md` at the repo root plus `docs/adr/`. See `docs/agents/domain.md`.
