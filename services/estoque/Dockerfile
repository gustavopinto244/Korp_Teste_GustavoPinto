# syntax=docker/dockerfile:1

FROM golang:1.27 AS build
WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/estoque ./cmd/api

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/estoque /estoque

EXPOSE 8081

ENTRYPOINT ["/estoque"]
