# Estimeet

**Fibonacci estimation for your backlog — live in a call, or on your own time.**

Estimeet is a planning-poker app for sizing anything your team needs to estimate. Topics come from a
Jira epic, an Azure DevOps feature, a GitHub milestone or the built-in composer, everyone plays a card
from the Fibonacci deck, the cards flip together, and the team agrees on a number.

It runs in two modes:

- **Synchronous** — the host walks the backlog one topic at a time. Everybody plays a card on the same
  story, the cards flip together, next topic. This is the classic refinement call.
- **Asynchronous** — every topic is open at once. Each teammate works through the backlog whenever they
  have time, and a topic flips as soon as the last member has played a card. Nobody has to be online at
  the same moment.

## Contents

[Quick start](#quick-start) · [Prerequisites](#prerequisites) · [Stack](#stack) · [Layout](#layout) ·
[Running it locally](#running-it-locally) · [Using the app](#using-the-app) ·
[Tests and checks](#tests-and-checks) · [Continuous integration](#continuous-integration) ·
[Configuration](#configuration) · [Deploying to estimeet.app](#deploying-to-estimeetapp) ·
[Publishing from GitHub](#publishing-from-github) · [Backlog import](#backlog-import-optional) ·
[How the rules work](#how-the-rules-work) · [Security notes](#security-notes) ·
[Troubleshooting](#troubleshooting) · [Licence](#licence)

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
  internal/source       One vocabulary over Jira, Azure DevOps and GitHub
  internal/secretbox    AES-256-GCM for tracker credentials at rest
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
   one title per line. Hosts can also import from Jira, Azure DevOps or GitHub (see below).
5. Estimate:
   - *synchronous* — the host drives with the **←** / **→** buttons or by clicking a backlog item;
     everyone plays a card on the current topic.
   - *asynchronous* — every topic is a card of its own; play them in any order. The **to estimate /
     all / done** filter and the progress bar track what you still owe.
6. When the cards flip you get the distribution, average, median, range and a suggested Fibonacci card.
   The host picks the agreed number, or hits **Vote again** to re-run the round.

Sessions survive a refresh: your token is kept in `localStorage`, and **Leave** clears it.

### Who is expected, and who is here

When you start a session you can say how many people are joining and, if you know them, list their
names — the **Who are you expecting?** block on the create form. You can change it later from the
sidebar with **+ who is expected**. The room then shows `Team (4 of 6)` plus a muted *Not here yet*
list, and synchronous sessions get an **At the table** strip above the deck showing everyone who has
joined, green once they have played their card and dimmed while they are offline.

The roster is a memory aid only. Names are matched against display names purely to grey out who is
missing; they never let anyone in and never keep anyone out.

### How long sessions are kept

A session — its participants, topics, votes and any tracker connection — is deleted once nobody has
touched it for `ESTIMEET_ROOM_RETENTION_DAYS` (30 by default). The clock runs from the last activity,
not from when the room was created, so a room in daily use is never removed. The value can be raised,
but never lowered below two weeks, so a team that estimates in one sprint can still open the session in
the next. A janitor sweeps every six hours and logs what it removed.

## Tests and checks

```powershell
# backend: static analysis and unit tests (domain statistics + service rules)
cd backend
go vet ./...
go test ./...

# frontend: type-check and production build
cd frontend
npm run build

# end-to-end: 45 assertions against a running API
pwsh ./scripts/smoke.ps1
```

The smoke test needs the API up. It covers both modes, the auto-reveal rules, observer handling,
host-only permissions, hidden votes before the reveal, and cross-room token isolation. Point it
elsewhere with `$env:ESTIMEET_API_BASE = 'https://estimeet.app/api'`.

## Continuous integration

`.github/workflows/ci.yml` runs on every push to `main` and every pull request, and does exactly what
you would do by hand:

| Job | What it runs |
| --- | --- |
| `backend` | `gofmt -l`, `go vet ./...`, `go test -race ./...` |
| `frontend` | `npm ci` then `npm run build` (which type-checks first) |
| `smoke` | starts the API, waits for `/api/health`, then runs `scripts/smoke.ps1` |
| `image` | builds the Dockerfile without pushing, so a broken image is caught in the PR |

The smoke job runs only after the other two pass, so a failure points straight at the layer that broke.

## Publishing from GitHub

Two more workflows put the app in front of people. Both need a one-off setting in the repository.

### The UI on GitHub Pages

`.github/workflows/pages.yml` builds the UI on every push to `main` and publishes it to Pages. Turn
Pages on once under **Settings → Pages → Build and deployment → Source: GitHub Actions**, then re-run
the workflow. Until that switch is flipped every run stops after a few seconds with *Get Pages site
failed*, and the workflow cannot flip it for you: creating a Pages site needs admin rights that the
job's `GITHUB_TOKEN` does not have.

Pages serves static files only — it cannot run the Go API or a WebSocket. Set the repository variable
**`ESTIMEET_API_BASE_URL`** (Settings → Secrets and variables → Actions → Variables) to the public
origin of your backend, for example `https://api.estimeet.app`, and add that Pages URL to the API's
`ESTIMEET_ALLOWED_ORIGINS` so CORS and the WebSocket handshake accept it. Without the variable the site
still builds, but every request goes back to Pages and fails.

| Variable | Default | Notes |
| --- | --- | --- |
| `ESTIMEET_API_BASE_URL` | *(empty)* | Public origin of the API the published UI should talk to. |
| `ESTIMEET_PAGES_BASE_PATH` | `/<repository>/` | Set to `/` when the site is served from a custom domain. |

The build also copies `index.html` to `404.html`, because Pages has no router and a deep link like
`/room/ABC123` would otherwise be a hard 404.

### The API as a container image

`.github/workflows/image.yml` builds the Dockerfile for `linux/amd64` and `linux/arm64` and pushes it
to the GitHub Container Registry on every push to `main` and every `v*` tag:

```powershell
docker pull ghcr.io/jatin-bhatia1/estimeet:main
```

It authenticates with the job's own `GITHUB_TOKEN`, so there are no registry credentials stored in the
repository. Run that image anywhere (Fly, Render, a VM, your own Kubernetes) and point
`ESTIMEET_API_BASE_URL` at it. Package visibility is set once under the repository's **Packages** page.

### Somewhere free to run the API

Pages cannot host the API, so it needs a home of its own. What Estimeet asks of a host is modest but
specific: **one instance** (the board is broadcast in memory), **WebSockets**, and **a disk that
survives a restart**, because the whole database is one SQLite file.

| Host | Cost | The catch |
| --- | --- | --- |
| **Render** free web service | free | Sleeps after 15 idle minutes and wakes in about a minute; the filesystem is wiped on every sleep and deploy, so sessions do not survive the night. |
| **Oracle Cloud Always Free** VM | free, indefinitely | A real machine with a real disk, so nothing is lost — but you install Docker and a TLS proxy yourself. |
| **Fly.io** | roughly $2/month | Closest fit: a 256MB machine plus a 1GB volume, auto-stopping when idle. There is no free allowance for new accounts. |

Render is the quickest way to make the published page work, and [render.yaml](render.yaml) already
describes the service: **New → Blueprint → this repository**. Render asks for `ESTIMEET_CONTACT_EMAIL`
on the first deploy and generates `ESTIMEET_SECRET` itself. Take the resulting
`https://estimeet-api.onrender.com` and set it as the repository variable `ESTIMEET_API_BASE_URL`, then
re-run the Pages workflow.

Hosts that hand the port to the process as `PORT` are supported: `ESTIMEET_ADDR` wins when it is set,
and `PORT` fills in when it is not.

### On a managed *App on Fargate* (Siemens SDC)

The platform owns the task definition, the load balancer and the certificate; the repository only has
to produce an image and push it to ECR. [.gitlab-ci.yml](.gitlab-ci.yml) does that — assume an AWS
role through OIDC, then build with Kaniko. There are no secrets to store: the OIDC token is minted per
job. Two **project CI/CD variables** name the target, and the pipeline stops early without them:

| Variable | Where it comes from |
| --- | --- |
| `AWS_GITLAB_ROLE_NAME` | the *Gitlab Repository* component — the role **name**, not the ARN |
| `ECR_REPO_URL` | the *App on Fargate* component — `<account>.dkr.ecr.<region>.amazonaws.com/<repo>` |

They live in **Settings → CI/CD → Variables** rather than in the file, because they carry an AWS
account id and this repository is mirrored to a public GitHub remote. Neither can be *Masked* — a role
name is too short and the URL contains dots and a slash — so mark them *Protected* and protect `main`.
`AWS_REGION` in the file has to match the region inside `ECR_REPO_URL`.

**One image, one ECR repository.** The Dockerfile builds the React UI and the Go API and ships them in
the same image, with the API serving the built files, so there is nothing to split into a second
container.

What this platform imposes, and how the repo answers it:

| It requires | Here |
| --- | --- |
| The container listens on **8000** — the auth sidecar proxies to `127.0.0.1:8000` | The image sets `PORT=8000` |
| An unauthenticated **`/health`** for the target group | Served at the root as well as `/api/health` |
| A **`latest`** tag to deploy | Pushed, alongside the commit sha |
| No persistent volume | Set `ESTIMEET_DB_URL`; see below |

Two things to know before deploying:

- **The task has no EFS volume**, so the SQLite file lives on the container's own disk and disappears
  with every deploy. Point `ESTIMEET_DB_URL` at a Postgres database, or accept that sessions are lost
  on each release.
- **The sidecar has to forward WebSocket upgrades**, or boards will fall back to nothing — the whole
  live view is one socket. If it terminates the connection instead, set `ESTIMEET_ALLOWED_ORIGINS` to
  the app's public URL first: the origin check compares against the browser's origin, and a sidecar
  that rewrites `Host` breaks the same-origin shortcut.

Two more things hold on any Fargate service, managed or not. **Run exactly one task**: the board is
broadcast from each process's memory, so a second task serves a different room to whoever lands on it,
and with SQLite a rolling deploy that briefly runs two tasks will corrupt the file. And **an idle
timeout below 25 seconds will drop live boards** — that is the server's WebSocket ping interval, and it
is chosen to sit comfortably inside an ALB's 60-second default.

Set `ESTIMEET_APP_BASE_URL` and `ESTIMEET_ALLOWED_ORIGINS` to the `subdomainUrl` the SDC console shows,
`ESTIMEET_SECRET` to something random and permanent — regenerating it invalidates every stored tracker
credential — and the database settings below.

### PostgreSQL instead of SQLite

Set `ESTIMEET_DB_URL` to a `postgres://` URL and the server uses that instead of the SQLite file;
leave it unset and nothing changes. The schema is created on connect, so an empty database is all the
server needs.

Managed platforms rarely hand over a finished URL, so the parts work too — they are only used when
`ESTIMEET_DB_URL` is empty, and only when a host is given:

| Variable | Default |
| --- | --- |
| `ESTIMEET_DB_HOST` | *(empty — nothing is assembled without it)* |
| `ESTIMEET_DB_PORT` | `5432` |
| `ESTIMEET_DB_NAME` | `estimeet` |
| `ESTIMEET_DB_USER` | *(empty)* |
| `ESTIMEET_DB_PASSWORD` | *(empty)* |
| `ESTIMEET_DB_SSLMODE` | `require` |

The password is URL-encoded on the way in, which is the reason to prefer the parts: a password
containing `@`, `/` or `?` pasted straight into a URL produces a DSN that fails to parse, or worse,
parses into the wrong host.

This exists for platforms where a file is the awkward part. On Fargate, SQLite means an EFS volume,
exactly one task, and no rolling deploys; with Postgres the task keeps no state, so it can be scaled
and redeployed normally. AWS's serverless relational option is **Aurora Serverless v2 (PostgreSQL)** —
a cluster with a minimum capacity of 0 ACU scales to zero when idle, and takes a few seconds to wake
on the first query, which the server waits out for up to 45 seconds at startup.

The two databases are kept behind one set of queries: they are written with `?` placeholders and
rewritten to `$1, $2` for Postgres, and the only real differences are the schema file, `GREATEST`
instead of SQLite's variadic `MAX`, and comparing names with `lower()` rather than `COLLATE NOCASE`.

Board state still lives in each process's memory, so more than one instance would mean participants
seeing different rooms. Postgres removes the storage reason to run one task, not the broadcast one.

## Configuration

Every backend setting can come from a **settings file** or from the **environment**, and the
environment always wins. Both use the same names, and every one has a working default for local
development, so the app runs with none of them set.

```powershell
cd backend
copy estimeet.conf.example estimeet.conf   # then edit it
```

The server reads `estimeet.conf` from its working directory, or from `ESTIMEET_CONFIG_FILE` if you
point that somewhere else. The format is `NAME = value`, one per line; `#` starts a comment, and quotes
keep surrounding spaces. The file is gitignored, because it is where the footer's contact address and
any secrets end up. The container image looks for it at `/data/estimeet.conf`, which is already a
volume — drop the file next to the database and restart.

```conf
ESTIMEET_CONTACT_EMAIL = feedback@estimeet.app
ESTIMEET_ISSUES_URL    = https://github.com/jatin-bhatia1/estimeet/issues/new/choose
```

| Variable | Default | Notes |
| --- | --- | --- |
| `ESTIMEET_CONFIG_FILE` | `estimeet.conf` | Settings file to read before the environment. Missing is fine. |
| `ESTIMEET_ADDR` | `:8090` | Listen address. Falls back to `PORT` when unset, for hosts that assign one. |
| `ESTIMEET_DB_PATH` | `data/estimeet.db` | SQLite file; created on first run. Ignored when `ESTIMEET_DB_URL` is set. |
| `ESTIMEET_DB_URL` | *(empty)* | `postgres://user:password@host:5432/estimeet?sslmode=require`. Switches the server to PostgreSQL. |
| `ESTIMEET_DB_HOST` / `_PORT` / `_NAME` / `_USER` / `_PASSWORD` / `_SSLMODE` | see above | Used to build the URL when `ESTIMEET_DB_URL` is empty and a host is set. |
| `ESTIMEET_ALLOWED_ORIGINS` | `http://localhost:5173,http://127.0.0.1:5173` | Comma-separated CORS **and** WebSocket origin allowlist. |
| `ESTIMEET_APP_BASE_URL` | `http://localhost:5173` | Where the Jira callback sends the browser back to. |
| `ESTIMEET_STATIC_DIR` | *(empty)* | Point at the built UI to serve it from the Go binary. |
| `ESTIMEET_SECRET` | dev fallback | Key for encrypting tracker credentials. **Required** when `ESTIMEET_ENV=production`; minimum 16 characters. |
| `ESTIMEET_ENV` | `development` | `production` makes `ESTIMEET_SECRET` mandatory. |
| `ESTIMEET_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error`. |
| `ESTIMEET_CONTACT_EMAIL` | *(empty)* | Address behind the footer's *Share an idea* link. The link is left out entirely when unset. |
| `ESTIMEET_ISSUES_URL` | `https://github.com/jatin-bhatia1/estimeet/issues` | Where the footer's *Report a problem* link points. |
| `ESTIMEET_ROOM_RETENTION_DAYS` | `30` | Days of inactivity before a session and everything in it is deleted. Values below 14 are raised to 14. |
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

docker run -d --name estimeet -p 8090:8000 `
  -v estimeet-data:/data `
  -e ESTIMEET_SECRET='<32+ random chars>' `
  -e ESTIMEET_APP_BASE_URL='https://estimeet.app' `
  -e ESTIMEET_ALLOWED_ORIGINS='https://estimeet.app' `
  -e JIRA_CLIENT_ID='...' `
  -e JIRA_CLIENT_SECRET='...' `
  -e JIRA_REDIRECT_URI='https://estimeet.app/api/jira/callback' `
  estimeet:latest
```

The image already sets `ESTIMEET_ENV=production`, `ESTIMEET_STATIC_DIR=/srv/web`,
`ESTIMEET_DB_PATH=/data/estimeet.db` and `ESTIMEET_CONFIG_FILE=/data/estimeet.conf`.
**Mount `/data` on a real volume** — that is the whole database.

**The container listens on 8000**, not the 8090 used in development: it sets `PORT=8000`, which
`ESTIMEET_ADDR` falls back to. Managed Fargate platforms put an authenticating sidecar in front of the
container and proxy to `127.0.0.1:8000`, and a host that assigns its own `PORT` still overrides it.

### Getting the settings to the deployed API

The footer's contact address and issues link are read by the **API**, not baked into the UI, so they
have to reach whatever host runs the container. Any of these works:

```powershell
# 1. Ship the file into the data volume, then restart.
docker cp backend/estimeet.conf estimeet:/data/estimeet.conf; docker restart estimeet

# 2. Mount it read-only from the host instead.
docker run -d -p 8090:8000 -v estimeet-data:/data `
  -v C:/etc/estimeet.conf:/data/estimeet.conf:ro estimeet:latest

# 3. No file at all - hand the same names to the platform.
docker run -d -p 8090:8000 -e ESTIMEET_CONTACT_EMAIL='...' -e ESTIMEET_ISSUES_URL='...' estimeet:latest
```

On a PaaS with no persistent filesystem (Fly, Render, Railway, Cloud Run, App Service) use the third
form: their environment or secrets UI, same variable names. Keep `ESTIMEET_SECRET` there too rather
than in the file — the file is convenient, not a secret store.

`GET /api/config` serves the contact address publicly, so it will be scraped. Prefer an alias or a
shared inbox over a personal address, or leave `ESTIMEET_CONTACT_EMAIL` empty and let the issues link
carry the feedback on its own.

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

## Backlog import (optional)

Estimeet can pull a backlog straight into the room from **Jira Cloud**, **Azure DevOps** or **GitHub**.
The host opens the **Backlog import** panel, picks a source and connects with an access token.

Whatever the source, the browsing is the same three steps, and each step is searched rather than
scrolled, because a real organisation has more projects and repositories than fit in a list:

| Source | Step 1 | Step 2 | Imported as topics |
| --- | --- | --- | --- |
| Jira Cloud | Project | Epic | the epic's stories |
| Azure DevOps | Project | Epic or feature | its child work items |
| GitHub | Repository | Milestone | the milestone's issues (pull requests excluded) |

Re-importing the same epic or milestone skips what is already in the backlog, so it is safe to run
again after grooming.

### Credentials are temporary, on purpose

A token is only needed to fill the backlog at the start of a session, so Estimeet does not keep one:

- it is encrypted with `ESTIMEET_SECRET` (AES-256-GCM) before it touches the database;
- only the room it was given to can use it, and it is never shown again;
- it is **deleted 24 hours after it was handed over**, and as soon as the session closes, whichever
  comes first — a refresh of an OAuth token does not extend that deadline;
- a background sweep removes expired rows every 15 minutes, and any read past the deadline behaves as
  if nothing had been stored.

The connect dialog says this before anything is typed, and the panel shows the exact deletion time
once a room is connected.

### Jira Cloud

1. Create a token at <https://id.atlassian.com/manage-profile/security/api-tokens>.
2. Fill in the site (`https://your-site.atlassian.net`, HTTPS and `*.atlassian.net` only), the
   Atlassian account email, and the token.

Alternatively, an operator can register an **OAuth 2.0 (3LO)** app so hosts approve with their own
Atlassian login instead of pasting a token:

1. Create the app at <https://developer.atlassian.com/console/myapps/>.
2. Add the **Jira API** permission with scopes `read:jira-work`, `read:jira-user` and `offline_access`.
3. Set the callback URL — local `http://localhost:8090/api/jira/callback`, production
   `https://estimeet.app/api/jira/callback`.
4. Export `JIRA_CLIENT_ID`, `JIRA_CLIENT_SECRET` and `JIRA_REDIRECT_URI`, then restart the API.

**Connect with Atlassian** then appears above the token form. The flow uses PKCE (S256) and the `state`
value is single-use.

### Azure DevOps

1. Create a personal access token with **Work Items (Read)** at
   `https://dev.azure.com/{organisation}/_usersSettings/tokens`.
2. Give Estimeet the organisation — either the bare name, or the `https://dev.azure.com/...` URL you
   copied from the browser.

### GitHub

1. Create a personal access token with the `repo` scope (or `public_repo` for public repositories only)
   at <https://github.com/settings/tokens>.
2. Paste it. The repository list is the one that token can see; typing `owner/name` in the search box
   also looks the repository up directly, so a repository outside the first hundred is still findable.

Every source inherits the permissions of the account behind the token, so an import sees exactly what
that person can see. The host-supplied parts of a URL are validated before any request leaves the
process: only `*.atlassian.net`, `dev.azure.com`/`*.visualstudio.com` organisation names and plain
`owner/name` repositories are accepted.

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
- JQL and WIQL values are quoted and escaped before they reach Jira or Azure DevOps.
- Tracker credentials are encrypted at rest and deleted 24 hours after they are given, or when the
  session closes.
- Upstream error bodies are never echoed back to the browser; only a short, sanitised summary is.
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

## Licence

Estimeet is released under the [PolyForm Perimeter License 1.0.1](LICENSE.md).

In plain terms: **use it for anything you like — personally, in your team, inside your company — but
do not sell it or run it as a product that competes with Estimeet.** You may read the source, run it,
change it and share your changes; you may not repackage it as your own planning-poker product or
service, paid or free. Keep the copyright notice with any copy you pass on.

Want to do something the licence does not allow? Ask — the answer may well be yes.
