# Testing

Prepare the retained Ruby compatibility implementation once, then run the Ruby suite, Go gateway suite, and Electron checks with:

```sh
mise run setup-ruby
mise run test
mise run desktop-check
```

The operational `mise run setup` path does not require Ruby or Bundler; `setup-ruby` is needed only while the legacy gateway and its compatibility tests remain through the final migration round.

Run Go race coverage for the concurrent stores, updater coordinator, and server with `go test -race ./internal/access ./internal/update ./internal/server`.

## Browser contract suite

The Playwright suite exercises Gripi only through a browser. Its fixtures use Pi's native JSONL session format, and its deterministic Pi replacement is an independent RPC subprocess. Browser specifications do not load Ruby code or call Sinatra directly.

Install Chromium once after installing npm dependencies:

```sh
npx playwright install chromium
```

Run the managed suite:

```sh
mise run e2e
```

The managed runner creates a temporary home, projects, Pi sessions, gateway state, and loopback port. It builds and starts a separate Go gateway process directly on the host and removes it afterward. It does not use or restart `gripi.service`, and it does not read or modify the user's Gripi or Pi session state.

The desktop Chromium project covers browser approval, session navigation, prompt streaming and persistence, new sessions, active-run controls, model settings, and extension UI. A focused mobile Chromium project covers the session drawer and a complete prompt lifecycle. Traces and screenshots are retained only for failed tests.

## External implementation

The same specifications can target another implementation:

```sh
GRIPI_E2E_BASE_URL=http://127.0.0.1:4567 \
GRIPI_E2E_ADMIN_PASSWORD=... \
npm run test:e2e:external
```

The target must be disposable and seeded with the contract fixtures. Generate them with:

```sh
node e2e/fixtures/seed.mjs /tmp/gripi-e2e-target
```

The command prints the fixture paths. Configure the target to use the printed session root and configured-directory file plus isolated gateway state paths. The fake must inherit `GRIPI_E2E_SESSIONS_ROOT` with the printed session root so it can persist newly created sessions.

For the Go gateway, configure the Pi command as a Node/script pair: `GRIPI_NODE=$(command -v node)` and `GRIPI_PI=$(pwd)/e2e/support/fake_pi.mjs`. An external implementation should invoke the same script with Node and pass Pi RPC arguments through unchanged. Browser approval may be enabled with the supplied admin password or disabled by the target.

Before any mutating specification runs, the setup project requires the visible `E2E Contract Ready` session and confirms that live status reports the `e2e/fixture-model` fake. This prevents accidentally running the suite against a personal gateway or a seeded target still connected to real Pi. The managed-server adapter is the only part which knows how to build and start the Go implementation, so browser scenarios remain implementation-independent.

Use one worker and no retries while the suite shares a target. Each scenario has its own seeded session, but retries could still encounter state left by the failed attempt.

## Optional real-Pi smoke

To make one real model request through the installed and authenticated Pi CLI:

```sh
npm run test:e2e:real
```

This is opt-in because it requires network access, credentials, model availability, and may incur provider cost. It uses the user's configured Pi agent directory for authentication and model settings, but stores the smoke-test session and all Gripi state under a temporary home. It is not run in CI.
