FROM golang:1.25-alpine AS build

RUN apk add --no-cache git ca-certificates

ENV GOPROXY=direct

WORKDIR /src

COPY agarthan/report_sync_service/go.work ./agarthan/report_sync_service/
COPY agarthan/report_sync_service/go.mod agarthan/report_sync_service/go.sum ./agarthan/report_sync_service/
COPY pkg/models/go.mod ./pkg/models/

RUN cd agarthan/report_sync_service && go mod download

COPY agarthan/report_sync_service ./agarthan/report_sync_service
COPY pkg/models ./pkg/models

RUN cd agarthan/report_sync_service && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/report_sync_service .

FROM alpine:3.20

RUN addgroup -S app && adduser -S app -G app

COPY --from=build /out/report_sync_service /usr/local/bin/report_sync_service

USER app

ENTRYPOINT ["/usr/local/bin/report_sync_service"]
