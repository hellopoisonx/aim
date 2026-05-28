#!/bin/sh
set -eu

: "${KAFKA_BOOTSTRAP:=kafka:9092}"
: "${KAFKA_PARTITIONS:=3}"
: "${KAFKA_REPLICATION_FACTOR:=1}"

TOPICS="
aim.user.events
aim-message-transfer
aim.conversation.events
aim.presence.events
aim.typing.events
aim.read_receipt.events
aim.attachment.uploaded
aim.attachment.parsed
"

for topic in $TOPICS; do
  echo "ensuring kafka topic: $topic"
  /opt/kafka/bin/kafka-topics.sh \
    --bootstrap-server "$KAFKA_BOOTSTRAP" \
    --create \
    --if-not-exists \
    --topic "$topic" \
    --partitions "$KAFKA_PARTITIONS" \
    --replication-factor "$KAFKA_REPLICATION_FACTOR"
done
