# ─── Stage 1: Build ──────────────────────────────────────
FROM node:20-alpine AS builder

WORKDIR /src

COPY ./src/package.json ./src/package-lock.json ./

# CMD below runs the webpack dev server (--hot), which needs webpack/webpack-cli/
# webpack-dev-server/babel-loader — all devDependencies. This container is the
# dev server itself, not a static production build, so devDependencies must stay.
# npm ci on the committed lock file (chantier 34): the dev server, the edge image
# and a rebuild from the public repository get the same dependencies.
RUN npm ci && npm cache clean --force

# ─── Stage 2: Runtime minimal ─────────────────────────────
FROM node:20-alpine AS runtime

WORKDIR /src

# Chromium système (musl/Alpine-compatible) pour playwright-core — Playwright's
# own bundled browser download only supports glibc and does not work here.
RUN apk add --no-cache chromium

# Copie des node_modules depuis le stage de build
COPY --from=builder /src/node_modules ./node_modules

# Copie du code source
COPY ./src /src

# Installation globale de nodemon pour le dev
RUN npm install -g nodemon

WORKDIR /src/app

EXPOSE 3000

# Health check — vérifie que le serveur webpack répond.
# 127.0.0.1 et pas localhost : webpack-dev-server écoute sur 0.0.0.0 (IPv4
# seulement) alors que le wget de busybox résout localhost en ::1 d'abord —
# le healthcheck échouait donc en permanence (constaté : 3078 échecs
# consécutifs) sur un serveur parfaitement fonctionnel.
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:3000/ || exit 1

CMD [ "npm", "start" ]