FROM nginx:1.27-alpine
COPY src/webapp/src/dist/ /usr/share/nginx/html/
COPY deploy/pi-tv/live-nginx.conf /etc/nginx/conf.d/default.conf
