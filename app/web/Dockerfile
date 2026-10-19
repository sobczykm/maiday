FROM nginxinc/nginx-unprivileged:1.30.6-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY index.html auth-bg.webp /usr/share/nginx/html/
EXPOSE 8080
