# One Dockerfile for all services. Build a Go service with
#   docker build --build-arg SERVICE=topology --target service .
# and the gateway (which also serves the frontend) with
#   docker build --build-arg SERVICE=gateway --target gateway .

# --- Protobuf code generation ---------------------------------------------
FROM bufbuild/buf:1.50.0 AS proto
WORKDIR /src
COPY buf.yaml buf.gen.yaml ./
COPY proto ./proto
RUN ["buf", "generate"]

# --- Frontend --------------------------------------------------------------
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN if [ -f package-lock.json ]; then npm ci; else npm install; fi
COPY web/ ./
RUN npm run build

# --- Go build --------------------------------------------------------------
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=proto /src/gen ./gen
ARG SERVICE
RUN test -n "$SERVICE" && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${SERVICE}

# --- Runtime ---------------------------------------------------------------
# Distroless: no shell, no package manager, runs as non-root.
FROM gcr.io/distroless/static-debian12:nonroot AS service
COPY --from=build /out/app /app
ENTRYPOINT ["/app"]

FROM service AS gateway
COPY --from=web /web/dist /static
ENV STATIC_DIR=/static
EXPOSE 8080
