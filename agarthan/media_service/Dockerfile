FROM golang:1.25-alpine AS build

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY agarthan/media_service/go.mod agarthan/media_service/go.work ./agarthan/media_service/
COPY pkg/models/go.mod ./pkg/models/

WORKDIR /app/agarthan/media_service
ENV GOPROXY=direct
RUN go mod download

WORKDIR /app
COPY agarthan/media_service/ ./agarthan/media_service/
COPY pkg/models/ ./pkg/models/

WORKDIR /app/agarthan/media_service
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/media_service .

FROM alpine:3.20

RUN addgroup -S app && adduser -S app -G app

COPY --from=build /out/media_service /usr/local/bin/media_service

USER app

EXPOSE 3002

ENTRYPOINT ["/usr/local/bin/media_service"]
