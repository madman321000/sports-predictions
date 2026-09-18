FROM golang:1.27.1-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/api ./cmd/api
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -o /api ./cmd/api
FROM alpine:3.22
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
COPY --from=build /api /usr/local/bin/api
USER app
EXPOSE 8080
ENV API_ADDRESS=:8080
ENTRYPOINT ["/usr/local/bin/api"]
