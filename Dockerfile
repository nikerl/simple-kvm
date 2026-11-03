# syntax=docker/dockerfile:1

# ---- Build stage ----
FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY src ./src
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/simple-kvm ./src

# ---- Runtime stage ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=build /out/simple-kvm .
COPY templates ./templates
COPY static ./static

EXPOSE 8090
CMD ["./simple-kvm"]
