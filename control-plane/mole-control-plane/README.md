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

For the local development configuration, use `--mole-url http://127.0.0.1:8080`.

Set these control-plane environment variables to enable provisioning:

```bash
TUNNEL_SERVER_URL=http://relay-private-address:9001
TUNNEL_SERVER_API_TOKEN=<same value as MOLE_SERVER_API_TOKEN on the relay>
```

The relay image requires `MOLE_SERVER_API_TOKEN`, `MOLE_PUBLIC_HOST`, and
`MOLE_CONTROL_PLANE_URL`. Its public tunnel-port range defaults to `10000-10100`
for both TCP and UDP; expose that range in the host firewall as needed. Keep the
management API on a private network or restrict access to the control plane.

## Local development

The repository includes safe local-only settings in `.env.dev`. Copy them to
the `.env` file each component reads at startup:

```bash
cp control-plane/mole-control-plane/.env.dev control-plane/mole-control-plane/.env
cp server/.env.dev server/.env
```

Start PostgreSQL and the two services in separate terminals:

```bash
cd control-plane/mole-control-plane && docker compose up -d
cd control-plane/mole-control-plane && go run ./cmd/api
cd server && go run ./cmd/server
```

The development files share a relay API token and bind everything to localhost.
Do not use their token values outside local development.

Clean up binary from the last build:
```bash
make clean
```
