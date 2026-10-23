FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY --from=web /src/internal/webui/dist/ ./internal/webui/dist/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /wiki ./cmd/wiki

FROM alpine:3.23
RUN apk add --no-cache ca-certificates tzdata && addgroup -g 10001 wiki && adduser -D -u 10001 -G wiki wiki && mkdir /data && chown wiki:wiki /data
COPY --from=build /wiki /usr/local/bin/wiki
USER wiki
ENV DATA_DIR=/data LISTEN_ADDR=0.0.0.0:8080
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -q -O /dev/null http://127.0.0.1:8080/readyz || exit 1
ENTRYPOINT ["wiki"]
CMD ["serve"]
