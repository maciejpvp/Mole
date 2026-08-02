# Stripe test and live environments

Mole uses separate Stripe credentials for local development and production.
Never mix a test key with a live key, and use separate databases so test-mode
Stripe customer IDs are not reused with live credentials.

## Local development

The frontend uses `mole-frontend/.env.development`, which contains a
`pk_test_...` publishable key. Configure the control plane's ignored
`control-plane/mole-control-plane/.env` with an `sk_test_...` secret key and
the signing secret from Stripe CLI:

```bash
stripe listen --forward-to localhost:8080/api/v1/billing/webhook
```

Copy the `whsec_...` value printed by Stripe CLI into the local control-plane
environment. Test cards are valid only with these test-mode credentials.

## Production

Activate the Stripe account before accepting live payments. Create a live
webhook destination in Stripe Workbench at:

```text
https://YOUR_DOMAIN/api/v1/billing/webhook
```

Subscribe it to:

- `setup_intent.succeeded`
- `setup_intent.setup_failed`
- `setup_intent.canceled`

Put the live secret key and the signing secret from that live endpoint in the
production host's ignored `.env` file:

```env
STRIPE_SECRET_KEY=sk_live_...
STRIPE_WEBHOOK_SECRET=whsec_...
```

The frontend publishable key is public, but it is fixed into the production
static bundle during the CI Docker build:

```bash
docker build \
  --build-arg VITE_STRIPE_PUBLISHABLE_KEY="$VITE_STRIPE_PUBLISHABLE_KEY" \
  -t maciekpvp/mole-frontend:latest \
  mole-frontend
```

Set the CI variable to `pk_live_...`, push the resulting image, and then run
the existing production Compose deployment. The production Compose file
requires both Stripe runtime variables instead of silently starting without
billing configuration.
