# syntax=docker/dockerfile:1

FROM golang:1.27 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/faturamento ./cmd/api

FROM alpine:3.20 AS final
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=build /out/faturamento /app/faturamento

EXPOSE 8082
USER appuser

HEALTHCHECK --interval=5s --timeout=3s --retries=5 \
  CMD wget -qO- http://localhost:8082/health || exit 1

ENTRYPOINT ["/app/faturamento"]
