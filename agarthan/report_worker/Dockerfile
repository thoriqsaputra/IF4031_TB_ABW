FROM golang:1.25-alpine AS build

RUN apk add --no-cache git ca-certificates

ENV GOPROXY=direct

WORKDIR /app

COPY agarthan/report_worker/go.mod agarthan/report_worker/go.sum agarthan/report_worker/go.work ./agarthan/report_worker/
COPY pkg/models/go.mod ./pkg/models/

WORKDIR /app/agarthan/report_worker
RUN go mod download

WORKDIR /app
COPY agarthan/report_worker/ ./agarthan/report_worker/
COPY pkg/models/ ./pkg/models/

WORKDIR /app/agarthan/report_worker
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/report_worker .

FROM alpine:3.20

RUN addgroup -S app && adduser -S app -G app

COPY --from=build /out/report_worker /usr/local/bin/report_worker

USER app

ENTRYPOINT ["/usr/local/bin/report_worker"]
