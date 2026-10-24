FROM node:24-alpine AS console
WORKDIR /src/console
COPY console/package.json console/package-lock.json ./
RUN npm ci
COPY console/ ./
RUN npm run build

FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=console /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/fleetflow ./cmd/fleetflow

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates python3 sumo sumo-tools \
    && test -x /usr/bin/sumo \
    && test -x /usr/bin/duarouter \
    && test -f /usr/share/sumo/tools/randomTrips.py \
    && /usr/bin/sumo --version \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build --chown=65532:65532 /out/fleetflow /fleetflow
COPY --chown=65532:65532 data/networks/jakarta.net.xml /app/data/networks/jakarta.net.xml
ENV PORT=8080
ENV ENV=production
ENV SUMO_HOME=/usr/share/sumo
ENV SUMO_BINARY=/usr/bin/sumo
ENV SUMO_DUAROUTER=/usr/bin/duarouter
ENV SUMO_RANDOM_TRIPS=/usr/share/sumo/tools/randomTrips.py
ENV SUMO_PYTHON=/usr/bin/python3
ENV SUMO_NETWORK=/app/data/networks/jakarta.net.xml
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/fleetflow"]
