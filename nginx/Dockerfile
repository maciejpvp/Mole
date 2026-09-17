FROM nginx:alpine

# Copy nginx configs directly into the image — avoids rootless Podman bind-mount
# permission issues. Consequence: any change in this directory requires
# rebuilding and pushing the image (scripts/build-and-push.sh --only nginx).
COPY nginx.conf /etc/nginx/nginx.conf
COPY conf.d/default.conf /etc/nginx/conf.d/default.conf
COPY partials/ /etc/nginx/partials/

# Runs before nginx starts; generates /etc/nginx/tls.d/ from the certificate
# state and keeps it in sync. See the header of the script for why.
COPY docker-entrypoint.d/40-mole-tls-sync.sh /docker-entrypoint.d/40-mole-tls-sync.sh
RUN chmod +x /docker-entrypoint.d/40-mole-tls-sync.sh \
    && mkdir -p /etc/nginx/tls.d /var/www/certbot
