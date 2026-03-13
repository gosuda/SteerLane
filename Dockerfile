# --- Dashboard build stage ---
FROM node:22-alpine AS dashboard

WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --ignore-scripts
COPY web/ .
RUN npm run build

# --- Go build stage ---
FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Use freshly built dashboard assets from the dashboard stage.
COPY --from=dashboard /src/web/build web/build/
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /steerlane ./cmd/steerlane

# --- Production stage ---
FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /steerlane /usr/local/bin/steerlane
COPY migrations /migrations

EXPOSE 8080
ENTRYPOINT ["steerlane"]
