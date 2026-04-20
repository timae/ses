# syntax=docker/dockerfile:1.6

# Multi-stage build: compile the share-server binary and ship it on top of a
# minimal distroless base. The binary is pure Go (stdlib only), so CGO can
# stay off and we get a fully static image.

FROM golang:1.25-alpine AS build
WORKDIR /src

# Cache modules separately from source.
COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath -ldflags="-s -w" \
      -o /out/share-server \
      ./cmd/share-server

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/share-server /app/share-server

# Persistent share data lives here — mount a volume at /data for durability.
ENV SHARE_DATA_DIR=/data/shares
VOLUME ["/data"]

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/share-server"]
