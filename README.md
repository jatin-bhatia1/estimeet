# Estimeet

**Fibonacci estimation for your backlog — live in a call, or on your own time.**

Estimeet is a planning-poker app for sizing anything your team needs to estimate. Topics come from a
Jira epic or from the built-in composer, everyone plays a card from the Fibonacci deck, the cards flip
together, and the team agrees on a number.

It runs in two modes:

- **Synchronous** — the host walks the backlog one topic at a time. Everybody plays a card on the same
  story, the cards flip together, next topic. This is the classic refinement call.
- **Asynchronous** — every topic is open at once. Each teammate works through the backlog whenever they
  have time, and a topic flips as soon as the last member has played a card. Nobody has to be online at
  the same moment.

## Contents

[Quick start](#quick-start) · [Prerequisites](#prerequisites) · [Stack](#stack) · [Layout](#layout) ·
[Running it locally](#running-it-locally) · [Using the app](#using-the-app) ·
[Tests and checks](#tests-and-checks) · [Configuration](#configuration) ·
[Deploying to estimeet.app](#deploying-to-estimeetapp) · [Jira Cloud](#jira-cloud-optional) ·
[How the rules work](#how-the-rules-work) · [Security notes](#security-notes) ·
[Troubleshooting](#troubleshooting)

## Quick start

```powershell
git clone git@github.com:jatin-bhatia1/estimeet.git
cd estimeet

# terminal 1 — API on http://localhost:8090
cd backend
go run ./cmd/server

# terminal 2 — UI on http://localhost:5173
cd frontend
npm install
npm run dev
```

Open <http://localhost:5173> and create a session. No account, no database server, no configuration.

## Prerequisites

| Tool | Version | Check |
| --- | --- | --- |
| Go | 1.24 or newer | `go version` |
| Node.js | 20 LTS or newer (24 recommended) | `node -v` |
| npm | 10 or newer | `npm -v` |

There is no separate database to install — SQLite is embedded via a pure-Go driver, so no cgo
toolchain is required either.

<details>
<summary>Installing the toolchain on Windows without admin rights</summary>

Both toolchains ship as portable zips, so no installer and no UAC prompt is needed:

```powershell
# Go
Invoke-WebRequest https://go.dev/dl/go1.26.5.windows-amd64.zip -OutFile $env:TEMP\go.zip
Expand-Archive $env:TEMP\go.zip -DestinationPath "$env:LOCALAPPDATA\Programs"
Rename-Item "$env:LOCALAPPDATA\Programs\go" "$env:LOCALAPPDATA\Programs\Go"

# Node
Invoke-WebRequest https://nodejs.org/dist/v24.18.1/node-v24.18.1-win-x64.zip -OutFile $env:TEMP\node.zip
Expand-Archive $env:TEMP\node.zip -DestinationPath $env:TEMP
Move-Item "$env:TEMP\node-v24.18.1-win-x64" "$env:LOCALAPPDATA\Programs\nodejs"

# add both to PATH for the current session
$env:PATH = "$env:LOCALAPPDATA\Programs\nodejs;$env:LOCALAPPDATA\Programs\Go\bin;$env:PATH"

# ...and permanently, for new terminals
[Environment]::SetEnvironmentVariable(
  'Path',
  "$env:LOCALAPPDATA\Programs\nodejs;$env:LOCALAPPDATA\Programs\Go\bin;" +
    [Environment]::GetEnvironmentVariable('Path', 'User'),
  'User')
```

</details>

## Stack

| Layer | Choice | Why |
| --- | --- | --- |
| API | Go 1.26 · chi · coder/websocket | Goroutine-per-connection fan-out is a natural fit for a live room; a single static binary to deploy. |
| Storage | SQLite (`modernc.org/sqlite`) | Pure Go, no cgo, no server to run. Rooms are small and mostly single-writer. |
| Web | React 19 · TypeScript · Vite 7 · Tailwind 4 | Fast HMR, tiny bundle, no runtime CSS cost. |
| Realtime | WebSocket, server-authoritative state | Every mutation returns the full per-viewer room state and broadcasts it, so clients never diverge. |

## Layout

```
backend/
  cmd/server            entry point
  internal/config       environment configuration
  internal/domain       pure rules: deck, room codes, statistics
  internal/store        SQLite persistence
  internal/service      application rules (permissions, voting, auto-reveal)
  internal/hub          per-room WebSocket registry and fan-out
  internal/api          HTTP routes, middleware, WebSocket handler
  internal/jira         Jira Cloud REST wrapper + OAuth 2.0 (3LO)
  internal/secretbox    AES-256-GCM for Jira credentials at rest
frontend/
  src/lib               API client, session storage, socket hook, types
  src/components        deck, boards, panels
  src/pages             home and room
scripts/smoke.ps1       end-to-end API check
Dockerfile              multi-stage build producing a single container
```

## Running it locally

The app is two processes in development: the Go API and the Vite dev server. Vite proxies `/api`
(including the WebSocket upgrade) to the API, so the browser only ever talks to port 5173.

### 1. Start the API

```powershell
cd backend
go run ./cmd/server
```

```
level=INFO msg="jira oauth disabled, api-token connections still available (set JIRA_CLIENT_ID, JIRA_CLIENT_SECRET, JIRA_REDIRECT_URI to enable)"
level=INFO msg="estimeet api listening" addr=:8090 db=data/estimeet.db
```

The SQLite file is created on first run at `backend/data/estimeet.db`. Delete that file to reset
everything.

### 2. Start the web UI

```powershell
cd frontend
npm install      # first time only
npm run dev
```

```
VITE v7.3.6  ready in 376 ms
➜  Local:   http://localhost:5173/
```

### From VS Code instead

**Terminal → Run Task…** and pick **dev** to start both at once, or **dev: api** / **dev: web**
individually. `.vscode/tasks.json` also has **test: backend**, **build: frontend** and **test: smoke**.

## Using the app

1. Open <http://localhost:5173>.
2. **Start a session** — give it a name, enter your display name, and pick **Synchronous** or
   **Asynchronous**. Leave *flip the cards automatically* on unless you want to reveal by hand.
3. Share the 6-character room code (or use **Share link**) with your team. They join with just a display
   name — no accounts. Anyone who is not estimating can tick **join as an observer**; observers see
   everything but are never counted as a missing vote.
4. Add topics with the composer in the sidebar — one at a time, or switch to **paste a list** and drop in
   one title per line. Hosts can also import a Jira epic (see below).
5. Estimate:
   - *synchronous* — the host drives with the **←** / **→** buttons or by clicking a backlog item;
     everyone plays a card on the current topic.
   - *asynchronous* — every topic is a card of its own; play them in any order. The **to estimate /
     all / done** filter and the progress bar track what you still owe.
6. When the cards flip you get the distribution, average, median, range and a suggested Fibonacci card.
   The host picks the agreed number, or hits **Vote again** to re-run the round.

Sessions survive a refresh: your token is kept in `localStorage`, and **Leave** clears it.

## Tests and checks

```powershell
# backend: static analysis and unit tests (domain statistics + service rules)
cd backend
go vet ./...
go test ./...

# frontend: type-check and production build
cd frontend
npm run build

# end-to-end: 30 assertions against a running API
pwsh ./scripts/smoke.ps1
```

The smoke test needs the API up. It covers both modes, the auto-reveal rules, observer handling,
host-only permissions, hidden votes before the reveal, and cross-room token isolation. Point it
elsewhere with `$env:ESTIMEET_API_BASE = 'https://estimeet.app/api'`.

## Configuration

All backend settings are environment variables. Every one has a working default for local development,
so the app runs with none of them set.

| Variable | Default | Notes |
| --- | --- | --- |
| `ESTIMEET_ADDR` | `:8090` | Listen address. |
| `ESTIMEET_DB_PATH` | `data/estimeet.db` | SQLite file; created on first run. |
| `ESTIMEET_ALLOWED_ORIGINS` | `http://localhost:5173,http://127.0.0.1:5173` | Comma-separated CORS **and** WebSocket origin allowlist. |
| `ESTIMEET_APP_BASE_URL` | `http://localhost:5173` | Where the Jira callback sends the browser back to. |
| `ESTIMEET_STATIC_DIR` | *(empty)* | Point at the built UI to serve it from the Go binary. |
| `ESTIMEET_SECRET` | dev fallback | Key for encrypting Jira credentials. **Required** when `ESTIMEET_ENV=production`; minimum 16 characters. |
| `ESTIMEET_ENV` | `development` | `production` makes `ESTIMEET_SECRET` mandatory. |
| `ESTIMEET_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error`. |
| `JIRA_CLIENT_ID` | — | Optional. Adds **Connect with Atlassian** (OAuth) next to the API-token form; all three are needed. |
| `JIRA_CLIENT_SECRET` | — | |
| `JIRA_REDIRECT_URI` | `http://localhost:8090/api/jira/callback` | Must match the Atlassian app registration exactly. |

Generate a production secret with:

```powershell
[Convert]::ToBase64String((1..32 | ForEach-Object { Get-Random -Max 256 }))
```

## Deploying to estimeet.app

In production the Go binary serves the built UI itself, so there is **one process, one port and one
origin** — which also means no CORS and no cross-origin WebSocket to worry about.

### With Docker (recommended)

```powershell
docker build -t estimeet:latest .

docker run -d --name estimeet -p 8090:8090 `
  -v estimeet-data:/data `
  -e ESTIMEET_SECRET='<32+ random chars>' `
  -e ESTIMEET_APP_BASE_URL='https://estimeet.app' `
  -e ESTIMEET_ALLOWED_ORIGINS='https://estimeet.app' `
  -e JIRA_CLIENT_ID='...' `
  -e JIRA_CLIENT_SECRET='...' `
  -e JIRA_REDIRECT_URI='https://estimeet.app/api/jira/callback' `
  estimeet:latest
```

The image already sets `ESTIMEET_ENV=production`, `ESTIMEET_STATIC_DIR=/srv/web` and
`ESTIMEET_DB_PATH=/data/estimeet.db`. **Mount `/data` on a real volume** — that is the whole database.

### Without Docker

```powershell
cd frontend; npm ci; npm run build
cd ..\backend; go build -trimpath -o estimeet.exe ./cmd/server

$env:ESTIMEET_ENV             = 'production'
$env:ESTIMEET_SECRET          = '<32+ random chars>'
$env:ESTIMEET_STATIC_DIR      = '..\frontend\dist'
$env:ESTIMEET_APP_BASE_URL    = 'https://estimeet.app'
$env:ESTIMEET_ALLOWED_ORIGINS = 'https://estimeet.app'
.\estimeet.exe
```

### DNS and TLS

Point `estimeet.app` at the host and terminate TLS in front of the app (Caddy, nginx, Cloudflare, or
your platform's load balancer). The reverse proxy **must forward WebSocket upgrades** on
`/api/rooms/*/ws` — the live board stops updating otherwise. With Caddy that is the whole config:

```caddyfile
estimeet.app {
	encode zstd gzip
	reverse_proxy 127.0.0.1:8090
}
```

### Post-deploy check

```powershell
$env:ESTIMEET_API_BASE = 'https://estimeet.app/api'
pwsh ./scripts/smoke.ps1
```

## Jira Cloud (optional)

Estimeet can pull the stories of an epic straight into the backlog. The host opens the **Jira** panel
inside a room and connects in one of two ways.

### API token (no server configuration)

This works out of the box, on any deployment.

1. Create a token at <https://id.atlassian.com/manage-profile/security/api-tokens>.
2. In the room, click **Connect to Jira** and fill in:
   - **Site** — `https://your-site.atlassian.net` (only `*.atlassian.net` sites are accepted, over HTTPS)
   - **Email** — the Atlassian account the token belongs to
   - **API token** — the value from step 1
3. The credentials are verified against the site before anything is stored.

The token inherits the permissions of that account, so imports see exactly the issues that person can see.

### OAuth 2.0 (3LO)

Nicer for shared deployments: each host approves the app with their own Atlassian login, and no token
has to be pasted around.

1. Create an **OAuth 2.0 (3LO)** app at <https://developer.atlassian.com/console/myapps/>.
2. Add the **Jira API** permission with scopes `read:jira-work`, `read:jira-user` and `offline_access`.
3. Set the callback URL:
   - local: `http://localhost:8090/api/jira/callback`
   - production: `https://estimeet.app/api/jira/callback`
4. Export the three variables and restart the API:

   ```powershell
   $env:JIRA_CLIENT_ID     = '...'
   $env:JIRA_CLIENT_SECRET = '...'
   $env:JIRA_REDIRECT_URI  = 'http://localhost:8090/api/jira/callback'
   go run ./cmd/server
   ```

**Connect with Atlassian** then appears in the connect dialog alongside the API-token form.

### Importing

Once connected, narrow by project, search for the epic by name or key, tick the stories you want and
import. Re-importing the same epic skips issues that are already in the backlog, so it is safe to run
again after grooming.

API tokens, access tokens and refresh tokens are all encrypted with `ESTIMEET_SECRET` before they touch
the database, the OAuth flow uses PKCE (S256), and the `state` value is single-use.

## How the rules work

**The deck** is `0 1 2 3 5 8 13 21 34 55 89` plus `?` (no idea) and `☕` (need a break). The two special
cards are counted in the distribution but excluded from the average, median and range.

**Reveal.** The host can always reveal manually. With auto-reveal on:

- *synchronous* — flips when every **currently connected** player has voted on the current topic;
- *asynchronous* — flips when every **member** of the room has voted, connected or not.

Observers never count toward either total, and card values are simply absent from the payload until a
topic is revealed — a curious teammate cannot read them out of the network tab.

**Finalising.** After the reveal the host picks the agreed number (the UI suggests the nearest
Fibonacci card to the average). "Vote again" clears the round and reopens the topic.

## Security notes

- Session tokens are random, stored **hashed** (SHA-256), and scoped to a single room.
- The WebSocket token travels in `Sec-WebSocket-Protocol`, never in the query string.
- Unauthenticated endpoints (create room, join room) are rate limited per IP.
- JSON bodies are capped at 1 MiB and reject unknown fields.
- JQL values are quoted and escaped before they reach Jira.
- The OAuth callback redirects only to the configured `ESTIMEET_APP_BASE_URL`.

## Troubleshooting

| Symptom | Cause and fix |
| --- | --- |
| `connection forcibly closed` on port 8090 | Something else owns the port. Check with `Get-NetTCPConnection -LocalPort 8090 -State Listen`, then set `ESTIMEET_ADDR=":9000"` and update `target` in `frontend/vite.config.ts`. |
| Header shows **offline** / board stops updating | The WebSocket was blocked. In production make sure the reverse proxy forwards upgrades; locally make sure the API is actually running. |
| `ESTIMEET_SECRET must be set outside development` | `ESTIMEET_ENV=production` requires an explicit 16+ character secret. |
| Jira panel says *not configured* | All three of `JIRA_CLIENT_ID`, `JIRA_CLIENT_SECRET`, `JIRA_REDIRECT_URI` must be set, and the API restarted. |
| Jira returns `invalid redirect_uri` | `JIRA_REDIRECT_URI` must match the Atlassian app registration character for character. |
| `npm install` warns about blocked install scripts | npm 12 blocks postinstall by default. Run `npm install-scripts approve esbuild` if the build fails. |
| Want a clean slate | Stop the API and delete `backend/data/estimeet.db`. |
