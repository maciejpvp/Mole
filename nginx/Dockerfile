FROM nginx:alpine

# Copy nginx configs directly into the image — avoids rootless Podman bind-mount permission issues
COPY nginx.conf /etc/nginx/nginx.conf
COPY conf.d/default.conf /etc/nginx/conf.d/default.conf
