# Deploying Mole

The whole deploy, on a prepared server with a filled `.env`:

```sh
docker compose -f docker-compose.prod.yml up -d
```

TLS certificates, database migrations, and the first admin account are all handled automatically. For Stripe-specific setup see [STRIPE_DEPLOYMENT.md](STRIPE_DEPLOYMENT.md).

## 1. Prerequisites

**DNS must be correct before the first `up -d`.** Point an A record for `MOLE_DOMAIN` at the server's public IP. Certbot validates over HTTP-01, so a missing or stale record means no certificate.

Open these ports to the internet:

| Port | Protocol | Purpose |
|---|---|---|
| 80 | TCP | HTTP → redirects to HTTPS once a certificate exists; also serves the ACME challenge |
| 443 | TCP | The site and the REST API |
| 9000 | TCP | Relay control channel (`MOLE_CONTROL_PORT`) — raw TCP, not HTTP |
| 10000–10100 | TCP **and** UDP | Public tunnel data ports (`MOLE_TUNNEL_PORT_MIN`/`MAX`) |

## 2. Prepare the server (once)

```sh
sudo bash scripts/setup-server.sh
```

Adds 2 GB of swap and pins Docker's address pool to `10.200.0.0/16` so it cannot collide with an AWS VPC's `172.31.x.x`.

## 3. Configure

```sh
cp .env.example .env
$EDITOR .env
```

Set `MOLE_DOMAIN` and `CERTBOT_EMAIL` first — `GOOGLE_REDIRECT_URL`, `GOOGLE_FRONTEND_URL`, `CORS_ALLOWED_ORIGINS`, and `MOLE_PUBLIC_HOST` all default to values derived from `MOLE_DOMAIN`, so you only override them if the relay lives on a different hostname.

Set `BOOTSTRAP_ADMIN_EMAIL` to the Google account that should become the first administrator.

While testing, set `CERTBOT_EXTRA_ARGS=--staging` to avoid Let's Encrypt's rate limits.

## 4. Build and push images

**From a development machine, never on the server** — the frontend build runs `npm ci` plus Vite and will not survive a 2 GB instance.

```sh
export VITE_STRIPE_PUBLISHABLE_KEY=pk_live_...   # or put it in .env.build
./scripts/build-and-push.sh
```

The last line printed is `MOLE_IMAGE_TAG=<tag>`. Put that in the server's `.env`. Pinning a real tag also fixes the pull problem: with `latest`, `docker compose up -d` reuses whatever image is already on the host.

## 5. Deploy

```sh
docker compose -f docker-compose.prod.yml up -d
```

The site is reachable over plain HTTP immediately. Within a minute or two certbot issues a certificate, nginx picks it up on its own, and HTTP starts redirecting to HTTPS. Watch it happen:

```sh
docker logs -f mole-certbot
docker logs -f mole-proxy    # look for "[mole-tls-sync] nginx reloaded"
```

## 6. First admin

Sign in with the Google account named in `BOOTSTRAP_ADMIN_EMAIL`. It is promoted to administrator during that sign-in — no restart, no manual SQL. If the account already existed before you set the variable, it is promoted at control-plane startup instead.

New Google accounts land on the `pending` plan. Being an administrator does not grant a plan, so pick one separately.

## Upgrading

```sh
./scripts/build-and-push.sh          # on your dev machine; note the new tag
# on the server: update MOLE_IMAGE_TAG in .env, then
docker compose -f docker-compose.prod.yml up -d
```

Database migrations are embedded in the control-plane binary and run automatically at startup. There is no separate migration step and no down-migration path.

## Troubleshooting

**nginx config changes appear to do nothing.** The configs under `nginx/` are baked into the `mole-nginx` image, not mounted. Editing them on the server has no effect — rebuild and push:

```sh
./scripts/build-and-push.sh --only nginx
```

**Switching certbot from staging to production.** Once a staging lineage exists, certbot keeps renewing it and your browser keeps rejecting the cert. Delete the lineage first:

```sh
docker compose -f docker-compose.prod.yml run --rm --entrypoint certbot certbot delete --cert-name $MOLE_DOMAIN
docker compose -f docker-compose.prod.yml restart certbot
```

**No certificate is being issued.** Check that DNS resolves to this server, that port 80 is open from the internet, and that `curl http://$MOLE_DOMAIN/.well-known/acme-challenge/test` reaches nginx rather than timing out. `docker logs mole-certbot` names the specific validation failure. Certbot retries every 15 minutes.

**Google sign-in redirects but the session does not stick.** `GOOGLE_COOKIE_SECURE` defaults to `true`, so the cookie is dropped over plain HTTP. Confirm the certificate issued and that you are on `https://`.

**Behind Cloudflare or an ALB.** `nginx/partials/app-locations.conf` sets `X-Forwarded-Proto $scheme`, which is correct when nginx itself terminates TLS but overwrites the real value when something upstream does. In that setup, replace it with a `map $http_x_forwarded_proto` and rebuild the nginx image. Note that Cloudflare's proxy handles HTTP only — port 9000 and the tunnel port range must bypass it.

**Reducing the published port count.** The default tunnel range publishes 202 host ports (101 TCP + 101 UDP), each with its own `docker-proxy` process and iptables rules. Narrowing `MOLE_TUNNEL_PORT_MAX` is the safe lever. Setting `"userland-proxy": false` in `/etc/docker/daemon.json` removes the proxy processes entirely, but it has known UDP hairpinning quirks and these are UDP tunnels — test before adopting it.
