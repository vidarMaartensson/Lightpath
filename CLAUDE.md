# CLAUDE.md

Guidance for AI agents working in this repository. Read it before changing anything.

## What this is

Lightpath computes which customers lose service when sites in an open access fibre network fail.
Three Go services (topology, impact, gateway) talk gRPC; the browser talks GraphQL to the gateway.
See `README.md` for the architecture and the reasoning behind it.

## How to work

1. **Plan first.** For anything beyond a one-line fix, write a short plan and wait for approval.
   Say which packages you will touch and which tests you will add.
2. **Ask when the domain is unclear.** If a request touches network behaviour (what counts as
   affected, rerouted, redundant), ask instead of guessing. Wrong answers here look plausible.
3. **Small steps.** One concern per change. Run the tests after each step, not only at the end.

## Rules

- **Every feature ships with tests.** No exceptions:
  - domain logic (`internal/network`, `internal/impact`): table-driven tests, including the
    scenario from the request written out as a test case;
  - anything with I/O or failure handling (`internal/impactsvc`, `internal/rpc`): tests for the
    failure modes, not just the happy path. Retries, bad data, unavailable dependencies;
  - frontend logic in `web/src/lib`: vitest. Components stay thin so the logic is testable.
- If you change how impact is computed, update `TestSeedScenarios` so the story the UI tells is
  pinned down.
- **`internal/impact` stays pure.** No I/O, no logging, no time. It takes a network and returns a
  result.
- **No business logic in the gateway or the frontend.** The gateway translates; the frontend
  draws. Numbers come from the impact service.
- **Never edit `gen/`.** Change `proto/`, run `make gen`. Protobuf changes must pass `buf lint` and
  must not break the wire format (CI runs `buf breaking` on pull requests).
- **Validate at boundaries.** Data from Postgres or another service goes through
  `network.Validate` before it is used.
- **Errors carry meaning.** Unknown input is `INVALID_ARGUMENT`, a missing dependency is
  `UNAVAILABLE`. User-facing messages say what happened and what to do, without internals.
- Keep the demo seed deterministic. Tests depend on its numbers.

## Commands

```bash
make setup   # first time: buf generate, go mod tidy, npm install
make test    # all tests; must pass before you say you are done
make e2e     # browser tests (Playwright); run when you change the UI
make lint    # buf lint, go vet, tsc
make dev     # run locally without Docker
make up      # run in Docker
```

## Style

- Go: standard library first. `log/slog` for logging. Errors wrapped with `%w` and checked with
  `errors.Is`. Comments explain why, not what.
- TypeScript: strict mode, no `any`. Plain CSS with custom properties in `styles.css`; the design
  idea and tokens are described at the top of that file. UI copy is sentence case, plain words,
  and says what will happen.
