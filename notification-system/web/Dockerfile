# The console: built by Node, served by nginx, which also passes /v1 on to the API so
# that the browser talks to one origin.
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --no-fund --no-audit
COPY . .
RUN npm run build

FROM nginx:1.29-alpine
# Where nginx finds the API, and the DNS server it asks. The defaults are those of
# compose: Docker's resolver and the api service.
ENV DNS_RESOLVER=127.0.0.11 \
    API_URL=http://api:8080
# The image turns templates in this directory into /etc/nginx/conf.d when it starts.
COPY nginx.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
