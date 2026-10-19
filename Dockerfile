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

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build --chown=nonroot:nonroot /out/fleetflow /fleetflow
COPY --from=build --chown=nonroot:nonroot /src/data/graph.json /app/data/graph.json
ENV PORT=8080
ENV ENV=production
ENV GRAPH_PATH=data/graph.json
EXPOSE 8080
ENTRYPOINT ["/fleetflow"]
CMD ["-demo"]
