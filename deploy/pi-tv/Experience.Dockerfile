FROM python:3.11-slim-bookworm
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 \
    GAMEFLEX_CHROMIUM_EXECUTABLE=/usr/bin/chromium \
    XDG_CACHE_HOME=/tmp/gameflex-cache XDG_CONFIG_HOME=/tmp/gameflex-config
# Debian's Chromium packages include native linux/arm64 binaries and runtime libraries.
# All installation stays inside this image; DietPi itself is not modified.
RUN apt-get update \
    && apt-get install -y --no-install-recommends chromium xvfb x11vnc ca-certificates fonts-liberation \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --uid 1000 --create-home --shell /usr/sbin/nologin gameflex
RUN pip install --no-cache-dir Pillow==13.0.0 playwright==1.64.1 wsproto==1.3.2
WORKDIR /app
COPY tools/pi-tv/ /app/tools/pi-tv/
COPY deploy/pi-tv/compatibility.json /app/deploy/pi-tv/compatibility.json
COPY src/webapp/src/app/gameflex/experience/game-sources.json /app/src/webapp/src/app/gameflex/experience/game-sources.json
COPY deploy/pi-tv/chromium-manual-policy.json /etc/chromium/policies/managed/gameflex.json
USER gameflex
