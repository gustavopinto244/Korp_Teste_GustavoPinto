# syntax=docker/dockerfile:1

FROM golang:1.27 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/faturamento ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot AS final
WORKDIR /app
COPY --from=build /out/faturamento /app/faturamento

EXPOSE 8082
USER nonroot:nonroot
ENTRYPOINT ["/app/faturamento"]
