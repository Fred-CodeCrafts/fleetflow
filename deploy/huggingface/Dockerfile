# A Hugging Face Docker Space for FleetFlow.
#
# It builds the application from the public repository at a fixed tag, so the Space
# and the release cannot drift apart. Bump FLEETFLOW_TAG to deploy the next release.
#
# What it deliberately does not include: OSRM. A routing service needs a
# multi-gigabyte Jakarta extract and its own process; without it the Route Planner
# reports that routing is unavailable, which is the honest answer, and every other
# screen works. See deploy/free-hosting.md for the options that include it.

ARG FLEETFLOW_REPO=https://github.com/Fred-CodeCrafts/fleetflow.git
ARG FLEETFLOW_TAG=v0.1.6

FROM node:24-alpine AS source
ARG FLEETFLOW_REPO
ARG FLEETFLOW_TAG
RUN apk add --no-cache git \
    && git clone --depth 1 --branch "$FLEETFLOW_TAG" "$FLEETFLOW_REPO" /src
WORKDIR /src/console
RUN npm ci && npm run build

FROM golang:1.25-alpine AS build
COPY --from=source /src /src
WORKDIR /src
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/fleetflow ./cmd/fleetflow

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/fleetflow /fleetflow

# Spaces exposes this port; the application reads it from PORT.
ENV PORT=7860
ENV ENV=production
EXPOSE 7860

# Spaces runs the container as an unprivileged user. Nothing here writes to disk:
# with no DATABASE_URL the stores are in memory and the console is embedded.
USER 1000
ENTRYPOINT ["/fleetflow"]
