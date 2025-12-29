FROM golang:1.25-alpine AS build

WORKDIR /src

COPY agarthan/report_service/go.work ./agarthan/report_service/
COPY agarthan/report_service/go.mod agarthan/report_service/go.sum ./agarthan/report_service/
COPY pkg/middleware/go.mod ./pkg/middleware/
COPY pkg/models/go.mod ./pkg/models/

RUN cd agarthan/report_service && go mod download

COPY agarthan/report_service ./agarthan/report_service
COPY pkg/middleware ./pkg/middleware
COPY pkg/models ./pkg/models

RUN cd agarthan/report_service && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/report_service .

FROM alpine:3.20

RUN addgroup -S app && adduser -S app -G app

COPY --from=build /out/report_service /usr/local/bin/report_service

USER app

EXPOSE 3001

ENTRYPOINT ["/usr/local/bin/report_service"]
