# Mole

Mole exposes a service running on a private machine at a public address. You run a
small client next to the service; the client dials out to the Mole server and keeps
one connection open. Traffic arriving at the public port is carried back over that
connection, so the machine hosting the service needs no inbound firewall rule, no
port forwarding, and no public IP.

Unlike most tunnelling tools, Mole is not HTTP-specific: a tunnel is a raw **TCP** or
**UDP** port. Game servers, SSH, databases and web servers all work the same way.

Mole is a full self-hostable service rather than a single binary: accounts (Google
sign-in), plans with monthly minute and transfer quotas enforced live, Stripe card
verification, a web UI, and an admin panel.

---

## Table of contents

- [How it works](#how-it-works)
- [Repository layout](#repository-layout)
- [Running it locally](#running-it-locally)
- [Creating and using a tunnel](#creating-and-using-a-tunnel)
- [HTTP API](#http-api)
- [Plans, quotas and enforcement](#plans-quotas-and-enforcement)
- [Wire protocol](#wire-protocol)
- [Configuration](#configuration)
- [Tests](#tests)
- [Production deployment](#production-deployment)

---

## How it works

### The pieces

| Component | Path | What it is |
|---|---|---|
| **Client** | [client/](client/) | Go binary, stdlib only. Runs next to the private service. |
| **Control plane** | [control-plane/mole-control-plane/](control-plane/mole-control-plane/) | Go HTTP API (chi) **and** the relay, in one process. Owns Postgres. |
| **Frontend** | [mole-frontend/](mole-frontend/) | React 19 + Vite SPA, styled as a draggable-window desktop. |
| **Proxy** | [nginx/](nginx/) | TLS termination, HTTP routing, and a TCP stream proxy for the control port. |
| **Database** | Postgres 17 | Users, sessions, plans, tunnels, billing records. |

The relay used to be a separate service; commit `e7fc6ea` merged it into the control
plane. It is still a self-contained package ([internal/relay/](control-plane/mole-control-plane/internal/relay/))
that knows nothing about HTTP or SQL — the control plane hands it callbacks
([server.go:67-86](control-plane/mole-control-plane/internal/server/server.go#L67-L86))
so usage and connection-status updates are persisted without an internal HTTP hop.

### The data path

```
                    ┌──────────────────────── Mole server ────────────────────────┐
                    │                                                             │
  public user       │   nginx :443 ──► control plane API :8080 ──► Postgres       │
       │            │   nginx :9000 ─┐                                            │
       │            │                │  (stream proxy)                            │
       ▼            │                ▼                                            │
 :10000-:10100 ─────┼──►  relay  ◄── control port :9000                           │
   (TCP/UDP)        │      ▲                                                      │
                    └──────┼──────────────────────────────────────────────────────┘
                           │  1. client dials out, sends token  ── control conn ──┐
                           │  2. relay signals "new connection"                   │
                           │  3. client dials a second conn for that session      │
                           ▼                                                      │
                     ┌───────────────┐                                            │
                     │  mole client  │ ◄──────────────────────────────────────────┘
                     └───────┬───────┘
                             │  4. bridges to the local service
                             ▼
                      127.0.0.1:25565  (or whatever you exposed)
```

Step by step, for a TCP tunnel:

1. You create a tunnel in the UI (or via `POST /api/v1/tunnels`). The control plane
   allocates a free public port from `MOLE_TUNNEL_PORT_MIN..MAX`, tells the relay to
   bind a listener on it, stores the row, and returns a one-time **connection token**.
   Only the SHA-256 digest of that token is persisted.
2. You start the client with the token. It calls `POST /api/v1/tunnels/connect` to
   learn its protocol, local target, and the relay's control address, then opens a
   long-lived TCP connection to the control port and authenticates with the token.
   The tunnel's status flips to `active`.
3. When someone connects to the public port, the relay sends a one-byte signal
   (`0x01`) down the control connection.
4. The client opens a *second* connection to the control port, tagged as a data leg.
   The relay pairs it with the waiting public connection and copies bytes both ways.
5. The client connects to the local service and bridges the two.

UDP works the same way up to step 3, but uses a single multiplexed bridge connection
instead of one leg per session: the relay wraps each datagram in a frame carrying the
source address, and the client keeps a per-source socket to the local service
([udp_bridge.go](client/pkg/client/udp_bridge.go)).

If the control connection drops, the client reconnects with exponential backoff
(1s → 30s). If the relay answers the handshake with `0xFF`, the token is dead — the
tunnel was deleted or stopped — and the client exits instead of spinning
([agent.go:207-211](client/pkg/client/agent.go#L207-L211)).

### Restart behaviour

The relay keeps all tunnel state in memory. On startup, `tunnel.Service.Restore`
rebuilds the registry from Postgres *before* the control listener starts accepting, so
a reconnecting client never races a half-built registry
([service.go:236](control-plane/mole-control-plane/internal/tunnel/service.go#L236)).
A tunnel whose old port is no longer free is rebound to another one and the new port
is written back.

---

## Repository layout

```
client/                          Go tunnel client
  cmd/client/main.go             flag parsing, signal handling
  pkg/client/agent.go            control connection, reconnect loop, signal dispatch
  pkg/client/tcp_bridge.go       one data leg ↔ local TCP service
  pkg/client/udp_bridge.go       framed multiplexing for UDP
  pkg/client/control_plane.go    GET config from /api/v1/tunnels/connect

control-plane/mole-control-plane/
  cmd/api/main.go                startup, graceful shutdown
  internal/server/               chi router, middleware, per-domain HTTP handlers
  internal/relay/                the relay engine — listeners, bridging, metering
  internal/tunnel/               tunnel lifecycle + quota transactions
  internal/user/                 accounts, Google sign-in, sessions, profile
  internal/billing/              Stripe SetupIntents and webhooks
  internal/admin/                admin user listing and mutations
  internal/database/             connection + embedded SQL migrations

mole-frontend/src/
  windows/                       one component per app window (tunnels, admin, …)
  hooks/                         react-query hooks, one per endpoint
  lib/api.ts                     axios client and endpoint wrappers

nginx/                           proxy image: TLS sync entrypoint, stream proxy
scripts/                         setup-server.sh, build-and-push.sh
```

---

## Running it locally

### Prerequisites

Go 1.25+, Node 22+, Docker (for Postgres), and a Google OAuth client. Stripe test
keys are optional unless you want to exercise the card-validation flow.

### 1. Control plane

```sh
cd control-plane/mole-control-plane
cp .env.example .env
$EDITOR .env
```

Two values need attention beyond the Google credentials:

- `MOLE_SERVER_STATE_DIR` defaults to `/var/lib/mole-server`, which an unprivileged
  user cannot create. Point it at something writable, e.g. `./tmp/state`. If the
  directory cannot be created the relay refuses to start — the API still comes up,
  but tunnel creation returns `503`.
- `CORS_ALLOWED_ORIGINS` and `GOOGLE_FRONTEND_URL` name `http://localhost:3000`, while
  Vite serves on `5173` by default. Either run the frontend on 3000 (below) or change
  both variables.

Start Postgres and the service:

```sh
docker compose up -d          # postgres only
go run ./cmd/api              # or: make watch   (live reload via air)
```

Migrations are embedded in the binary and run at startup. There is no separate
migration step and no down-migration path.

Set `BOOTSTRAP_ADMIN_EMAIL` to your Google address to get an admin account on first
sign-in.

### 2. Frontend

```sh
cd mole-frontend
npm install
npm run dev -- --port 3000
```

`.env.development` points the SPA at `http://localhost:8080`.

### 3. Client

```sh
cd client
go build -o mole ./cmd/client
```

No dependencies — it builds anywhere Go does.

---

## Creating and using a tunnel

Sign in, open the tunnels window, and create a tunnel by choosing a protocol and the
local address to expose (for example `127.0.0.1:25565`). You get back a public
endpoint and a token, shown once.

Then, on the machine running the service:

```sh
./mole --mole-url https://your-domain.example --token <token>
```

Add `--debug` for per-connection logging. Locally, use
`--mole-url http://127.0.0.1:8080`.

The client stops on SIGINT/SIGTERM, on Ctrl+D when attached to a terminal, and when
the server rejects its token.

The same thing over the API:

```sh
curl -X POST https://your-domain.example/api/v1/tunnels \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"proto":"tcp","internal_address":"127.0.0.1:25565"}'
```

```json
{
  "id": "…",
  "proto": "tcp",
  "internal_address": "127.0.0.1:25565",
  "outbound_port": 10007,
  "endpoint": "your-domain.example:10007",
  "server_address": "your-domain.example:9000",
  "token": "…"
}
```

`internal_address` must be a literal `ip:port` — hostnames are rejected. Loopback and
private ranges are fine (the address is resolved on *your* machine, by the client),
but unspecified, multicast and link-local addresses are refused, which shuts the door
on cloud metadata endpoints like `169.254.169.254`. Add your own CIDRs or addresses
with `SSRF_BLOCKED_RANGES`. See `isSSRFForbiddenIP`
([service.go:611](control-plane/mole-control-plane/internal/tunnel/service.go#L611)).

---

## HTTP API

All routes are under `/api/v1`. Authenticated routes take
`Authorization: Bearer <access_token>`.

### Auth (Google only)

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/auth/google/start` | Begins OAuth; sets state + PKCE verifier cookies. |
| `GET` | `/auth/google/callback` | Google redirects here; redirects on to the frontend with a short-lived `google_code`. |
| `POST` | `/auth/google/exchange` | Trades that code for an access token. |

These three are rate-limited separately and more strictly than everything else
(`AUTH_RATE_LIMIT_RPS`, default 2/s).

### User

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/user/me` | Profile, plan limits, current usage, and all tunnels. Never returns tunnel tokens. |
| `GET` | `/plans` | The plan catalogue. |
| `GET` | `/events` | SSE stream. Sends an immediate `tunnel_update` snapshot, then one on every change, plus a comment ping every 15s. |

### Tunnels

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/tunnels` | Create. `429` when a plan limit is hit. |
| `DELETE` | `/tunnels/{tunnelID}` | Delete and unbind the public listener. |
| `POST` | `/tunnels/connect` | **Unauthenticated**, takes `{"token": …}`. The client's bootstrap call. |

### Billing

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/billing/card-validation/setup` | Creates a Stripe SetupIntent. |
| `POST` | `/billing/card-validation/confirm` | Confirms it and activates the free plan. |
| `POST` | `/billing/webhook` | **Unauthenticated**, signature-verified. Subscribe it to `setup_intent.succeeded`, `setup_intent.setup_failed`, `setup_intent.canceled`. |

### Admin

Requires an account with `is_admin`.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/admin/users` | Cursor-paginated. `search` is a username/email prefix; `sort` is one of `transfer`, `minutes`, `username`, `created_at`. |
| `PATCH` | `/admin/users/{userId}/plan` | `{"plan_id": 2}` |
| `POST` | `/admin/users/{userId}/reset-limits` | Zeroes the current period's counters. |
| `PATCH` | `/admin/users/{userId}/admin` | Grant or revoke admin. |
| `PATCH` | `/admin/users/{userId}/ban` | Ban or unban. |

---

## Plans, quotas and enforcement

Seeded plans ([001_create_plans.sql](control-plane/mole-control-plane/internal/database/migrations/001_create_plans.sql),
[007_billing.sql](control-plane/mole-control-plane/internal/database/migrations/007_billing.sql)):

| Plan | Concurrent tunnels | Minutes/month | Transfer/month |
|---|---|---|---|
| `pending` | 0 | 0 | 0 |
| `free` | 1 | 60 | 1 GiB |
| `premium` | 3 | 260 | 10 GiB |
| `unlimited` | ∞ | ∞ | ∞ |

`NULL` in the `plans` table means unlimited.

New Google accounts land on `pending`, which can do nothing. Validating a card via
Stripe (a SetupIntent — no charge) moves the account to `free`. Being an
administrator does not grant a plan; that is a separate change.

Enforcement runs on two clocks:

- **Live, in the relay.** Every byte copied is metered, and active tunnel-minutes are
  accumulated per user. When a user crosses a limit, their tunnels are killed
  immediately — this is the hard kill switch from commit `50641f7`.
- **Every 5 minutes, into Postgres.** `CollectUsage` → `ApplyUsage` writes the deltas
  in a serializable transaction that also rolls the monthly period over when it has
  expired, and returns the IDs of tunnels the relay must stop
  ([engine.go:239](control-plane/mole-control-plane/internal/relay/engine.go#L239)).

There is also a **global fuse**: `MOLE_GLOBAL_TRANSFER_LIMIT_BYTES` (default 5 TiB)
caps total transfer across the whole deployment, so a runaway tunnel cannot produce a
surprise bandwidth bill. The counter lives in `MOLE_SERVER_STATE_DIR/global-transfer.json`
and survives restarts. Once tripped, the relay refuses to start until you reset that
file.

---

## Wire protocol

Everything between the client and the relay runs over the single control port. Each
connection opens with the same handshake:

```
┌────────────┬──────────────┬──────┐
│ len (4, BE)│ token bytes  │ role │
└────────────┴──────────────┴──────┘
```

`role` is `0x00` control, `0x01` TCP data leg, `0x02` UDP bridge. Token length is
capped at 512 bytes; the token is compared by SHA-256 digest in constant time.

Signals, one byte each, sent by the relay on the **control** connection only:

| Byte | Meaning |
|---|---|
| `0x01` | A public TCP connection is waiting — open a data leg. |
| `0x02` | Open the UDP bridge. |
| `0xFF` | Token rejected. Stop reconnecting. |

Unknown bytes are ignored on both sides, which is what keeps older clients working
when a signal is added.

UDP datagrams are framed on the bridge connection:

```
┌────────────┬──────────────┬────────────┬─────────────┐
│ addr len   │ "ip:port"    │ payload len│ payload     │
└────────────┴──────────────┴────────────┴─────────────┘
```

Rejections are only written to control connections. Writing a byte to a data leg
would make the client forward it into the local service as payload.

---

## Configuration

The control plane reads its configuration from the environment
(`.env` is auto-loaded via godotenv). [.env.example](.env.example) at the repo root is
the production template; [control-plane/mole-control-plane/.env.example](control-plane/mole-control-plane/.env.example)
is the development one.

### Relay

| Variable | Default | Notes |
|---|---|---|
| `MOLE_CONTROL_PORT` | `9000` | Raw TCP, not HTTP. Must be reachable by clients. |
| `MOLE_TUNNEL_PORT_MIN` / `MAX` | `10000` / `10100` | Public tunnel ports. Each is published on **both** TCP and UDP, so the default range costs 202 host port bindings — narrow it on a small instance. |
| `MOLE_PUBLIC_HOST` | `MOLE_DOMAIN` | The hostname handed to clients. |
| `MOLE_GLOBAL_TRANSFER_LIMIT_BYTES` | 5 TiB | The global fuse. |
| `MOLE_SERVER_STATE_DIR` | `/var/lib/mole-server` | Where the fuse counter is persisted. |

### API

| Variable | Notes |
|---|---|
| `PORT` | HTTP listen port (8080 in compose; never published on the host). |
| `BLUEPRINT_DB_*` | Postgres host, port, database, user, password, schema. |
| `GOOGLE_CLIENT_ID` / `_SECRET` / `_REDIRECT_URL` / `_FRONTEND_URL` | OAuth. |
| `GOOGLE_COOKIE_SECURE` | `false` for plain-HTTP local development. |
| `BOOTSTRAP_ADMIN_EMAIL` | Promoted to admin at startup and on next sign-in. Clear it once the first admin exists. |
| `STRIPE_SECRET_KEY` / `STRIPE_WEBHOOK_SECRET` | Required by the production compose file. |
| `CORS_ALLOWED_ORIGINS` | Comma-separated. Never `*` — credentials are in play. |
| `RATE_LIMIT_RPS` / `_BURST` | Per-IP, all routes. Defaults 20 / 50. |
| `AUTH_RATE_LIMIT_RPS` / `_BURST` | Per-IP, `/auth/*` only. Defaults 2 / 5. |
| `MAX_REQUEST_BODY_BYTES` | Default 1 MiB. |
| `BLOCKED_IPS` / `ALLOWED_IPS` | Comma-separated IPs or CIDRs, applied to API callers. |
| `SSRF_BLOCKED_RANGES` | Extra IPs/CIDRs a tunnel's `internal_address` may not name. |

The frontend takes `VITE_CONTROL_PLANE_URL` (empty = same origin, which is what
production uses behind nginx) and `VITE_STRIPE_PUBLISHABLE_KEY`, both baked into the
static bundle at build time.

---

## Tests

```sh
cd control-plane/mole-control-plane
make test          # go test ./... -v
make itest         # database package only
```

The database tests use testcontainers and need a working Docker socket. The relay,
tunnel, user, admin and middleware packages are covered by unit tests that do not.

```sh
cd client && go test ./...
cd mole-frontend && npm run lint
```

---

## Production deployment

See **[DEPLOYMENT.md](DEPLOYMENT.md)** for the full walkthrough, and
**[STRIPE_DEPLOYMENT.md](STRIPE_DEPLOYMENT.md)** for keeping test and live Stripe
credentials apart. The short version:

```sh
sudo bash scripts/setup-server.sh              # once, on the server
./scripts/build-and-push.sh                    # on a dev machine — prints a tag
# put that tag in the server's .env as MOLE_IMAGE_TAG, then:
docker compose -f docker-compose.prod.yml up -d
```

TLS certificates (certbot, HTTP-01), migrations, and the first admin account are all
handled automatically. DNS must already point at the server before the first `up -d`,
or certificate issuance fails.

Ports that must be open: 80 and 443 (site, API, ACME challenge), 9000 (control
channel), and the tunnel range on both TCP and UDP.

Two things that surprise people:

- The nginx configs are **baked into the image**, not mounted. Editing them on the
  server does nothing; rebuild with `./scripts/build-and-push.sh --only nginx`.
- Pin a real `MOLE_IMAGE_TAG`. With `latest`, `docker compose up -d` happily reuses
  whatever image is already on the host.
