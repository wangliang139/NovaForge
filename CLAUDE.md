# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Architecture

Multi-language quantitative trading platform using microservices:

- `services/llt-trade-api` — Core trading API (Go, gRPC, sqlc)
- `services/llt-strategy-api` — Strategy execution engine (Go, gRPC, JS runtime, sqlc)
- `services/llt-data-api` — Market data distribution (Go, gRPC, Kafka/NATS publishers, sqlc)
- `services/llt-backoffice-gateway` — Admin gateway (Go, GraphQL via gqlgen)
- `services/llt-trade-py` — Python data processing service
- `frontend` — React/TypeScript SPA (Ant Design, UmiJS/Max framework, Apollo GraphQL client)
- `common/schema` — Protobuf definitions shared across all services
- `common/go` — Shared Go utilities

Service boundaries are enforced via protobuf. Each Go service uses sqlc for type-safe DB access and goverter for type conversion between layers.

## Commands

### Protobuf (run from `common/`)
```bash
make proto          # format, lint, breaking-change check, generate
make proto-breaking # generate without breaking-change detection
```

### Go services (run from `services/<service>/`)
```bash
make lint           # golangci-lint
make lint-fix       # golangci-lint --fix
make repo           # sqlc generate (regenerates DB layer)
make convert        # goverter gen (llt-data-api, llt-strategy-api)
make run            # lint + repo [+ convert where applicable]
```

`llt-backoffice-gateway` extras:
```bash
make build          # go build -o ./bin/app ./cmd/app.go
make gqlgen         # regenerate GraphQL schema/resolvers
```

`llt-trade-api` extras:
```bash
make proto          # regenerate proto (delegates to api/Makefile)
```

### Frontend (run from `frontend/`)
```bash
pnpm run start:dev  # dev server (no mock)
pnpm run build      # production build
pnpm run lint       # eslint + prettier + tsc
pnpm run test       # jest
```

## Code Generation

When modifying SQL queries, regenerate with `make repo` in the affected service.
When modifying proto schemas in `common/schema`, run `make proto` from `common/`.
When modifying GraphQL schema in `llt-backoffice-gateway`, run `make gqlgen`.
When modifying converter interfaces, run `make convert`.
