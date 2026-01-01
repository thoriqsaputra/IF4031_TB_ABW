#!/bin/sh
set -e

/__cacert_entrypoint.sh /etc/kafka/docker/run &
kafka_pid=$!

echo "Waiting for Kafka broker to be ready..."
ready=""
for i in $(seq 1 60); do
    if /opt/kafka/bin/kafka-broker-api-versions.sh --bootstrap-server localhost:9092 >/dev/null 2>&1; then
        ready="yes"
        break
    fi
    sleep 1
done

if [ "$ready" = "yes" ]; then
    /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 \
        --create --if-not-exists --topic submission.events \
        --replication-factor 1 --partitions 1
    /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 \
        --create --if-not-exists --topic report.published \
        --replication-factor 1 --partitions 1
else
    echo "Kafka broker did not become ready in time; skipping topic creation."
fi

wait "$kafka_pid"
