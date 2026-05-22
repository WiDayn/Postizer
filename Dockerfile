FROM golang:1.26.2-bookworm AS build

ARG POSTIZER_VERSION=dev
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X postizer/internal/http.AppVersion=${POSTIZER_VERSION}" -o /out/postizer ./cmd/postizer

FROM alpine:3.22

ARG POSTIZER_VERSION=dev
RUN apk add --no-cache ca-certificates tzdata curl tar gzip \
    && addgroup -S postizer \
    && adduser -S -G postizer postizer \
    && mkdir -p /app/content /app/media /app/runtime /usr/src/postizer \
    && chown -R postizer:postizer /app /usr/src/postizer

WORKDIR /app

COPY --chown=postizer:postizer --from=build /out/postizer /usr/src/postizer/postizer
COPY --chown=postizer:postizer --from=build /src/web /usr/src/postizer/web
COPY --chown=postizer:postizer --from=build /src/internal/bundles /usr/src/postizer/internal/bundles
COPY --chown=postizer:postizer --from=build /src/marketplace /usr/src/postizer/marketplace
COPY --chown=postizer:postizer scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
COPY --chown=postizer:postizer scripts/docker-self-update.sh /usr/local/bin/postizer-self-update
RUN printf '%s\n' "$POSTIZER_VERSION" > /usr/src/postizer/.postizer-version \
    && chmod 0755 /usr/local/bin/docker-entrypoint.sh /usr/local/bin/postizer-self-update

ENV POSTIZER_ADDR=:8080 \
    POSTIZER_SEED_VERSION=${POSTIZER_VERSION} \
    POSTIZER_SELF_UPDATE_COMMAND=/usr/local/bin/postizer-self-update \
    POSTIZER_SELF_UPDATE_INTERVAL=15m \
    POSTIZER_SELF_UPDATE_INITIAL_DELAY=5m

EXPOSE 8080
VOLUME ["/app/runtime", "/app/content", "/app/media"]

USER postizer

ENTRYPOINT ["docker-entrypoint.sh"]
