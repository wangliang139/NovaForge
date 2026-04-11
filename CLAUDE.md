# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Architecture

NovaForge is a **desktop-oriented monolith** for multi-exchange Web3 trading: a single Go backend and a React SPA.

- `server/` — Go application (GraphQL via gqlgen, strategy runtime, market connectors, sqlc repos, scheduled jobs). Entry: `cmd/app`.
- `frontend/` — React / TypeScript SPA (Ant Design Pro, UmiJS, Apollo Client for GraphQL).

The UI talks to the backend over **GraphQL** (same repo; local dev typically runs both processes). Persistence and domain logic live under `server/pkg/repos`, `server/pkg/entity`, resolvers, and strategy `pkg/strategy/`.

## Commands

### Go backend (from `server/`)

```bash
make lint           # golangci-lint
make lint-fix       # golangci-lint --fix
make repo           # sqlc generate (pkg/repos)
make convert        # goverter (pkg/converter)
make gqlgen         # regenerate GraphQL schema/resolvers
make build          # CGO_ENABLED=1 go build -o ./bin ./cmd/app
```

### Frontend (from `frontend/`)

```bash
pnpm run dev        # dev server
pnpm run build      # production build
pnpm run lint       # eslint + prettier + tsc
pnpm run test       # jest
```

## Code generation

- After changing SQL under `server/pkg/repos/*/`, run `make repo` in `server/`.
- After changing GraphQL schema, run `make gqlgen` in `server/`.
- After changing goverter interfaces, run `make convert` in `server/`.

## Database deployment

Aggregated PostgreSQL (and related) DDL for fresh environments lives under `deploy/`; keep it in sync with `server/pkg/repos/*/schema.sql` when schemas change.
