# Project mole-control-plane

One Paragraph of project description goes here

## Getting Started

These instructions will get you a copy of the project up and running on your local machine for development and testing purposes. See deployment for notes on how to deploy the project on a live system.

## MakeFile

Run build make command with tests
```bash
make all
```

Build the application
```bash
make build
```

Run the application
```bash
make run
```
Create DB container
```bash
make docker-run
```

Shutdown DB Container
```bash
make docker-down
```

DB Integrations Test:
```bash
make itest
```

Live reload the application:
```bash
make watch
```

Run the test suite:
```bash
make test
```

## Tunnel provisioning

## Administrator user listing

Administrators can list users with cursor pagination. Search is a
case-insensitive username or email prefix search; supported sort fields are
`transfer`, `minutes`, `username`, and `created_at`.

```http
GET /api/v1/admin/users?limit=50&search=alice&sort=transfer&direction=desc
Authorization: Bearer <administrator_access_token>
```

When `next_cursor` is present, pass it as `cursor` to retrieve the next page.
The endpoint never returns credentials or session tokens.

Administrators can change a user's plan by supplying a plan ID from the plan
catalog:

```http
PATCH /api/v1/admin/users/user-id/plan
Authorization: Bearer <administrator_access_token>
Content-Type: application/json

{"plan_id":2}
```

All authenticated users can list the available plans and their limits:

```http
GET /api/v1/plans
Authorization: Bearer <access_token>
```

Retrieve the authenticated account, current plan limits and usage, and all of
its tunnels:

```http
GET /api/v1/user/me
Authorization: Bearer <access_token>
```

Tunnel connection tokens are deliberately never returned by this endpoint.

Create a user session first, then create a tunnel with its bearer token:

```http
POST /api/v1/tunnels
Authorization: Bearer <access_token>
Content-Type: application/json

{"proto":"tcp","internal_address":"127.0.0.1:25565"}
```

The response includes the public `endpoint`, the relay `server_address`, and a
new `token`. Save the token securely and start the client with it:

```bash
mole-client --mole-url https://control-plane-address --token <token>
```

Delete a tunnel with the authenticated user's session token:

```http
DELETE /api/v1/tunnels/<tunnel_id>
Authorization: Bearer <access_token>
```

New tunnels are returned with status `inactive`. The relay changes the status
to `active` only after it authenticates the client connection, and changes it
back to `inactive` when that client disconnects.

For the local development configuration, use `--mole-url http://127.0.0.1:8080`.

The relay runs inside the control-plane process. Configure its public endpoint and listener range with `MOLE_PUBLIC_HOST`, `MOLE_CONTROL_PORT`, `MOLE_TUNNEL_PORT_MIN`, and `MOLE_TUNNEL_PORT_MAX`. Expose the TCP and UDP tunnel range in the host firewall as needed.

## Local development

The repository includes safe local-only settings in `.env.dev`. Copy them to
the `.env` file each component reads at startup:

```bash
cp control-plane/mole-control-plane/.env.dev control-plane/mole-control-plane/.env
```

Start PostgreSQL and the combined control-plane/relay service:

```bash
cd control-plane/mole-control-plane && docker compose up -d
cd control-plane/mole-control-plane && go run ./cmd/api
```

The control plane binds the API and relay listeners; do not expose development settings to production.

Clean up binary from the last build:
```bash
make clean
```
