#!/bin/sh
# Creates the AIM topics, their message types and the producer/consumer groups.
#
# Message types matter:
#   FIFO        - ordered delivery per message group (chat delivery, read receipts)
#   TRANSACTION - half message + local transaction + check-back (outbox replacement)
#   NORMAL      - fire-and-forget notifications
set -eu

: "${ROCKETMQ_NAMESRV:=rocketmq-namesrv:9876}"
: "${ROCKETMQ_CLUSTER:=DefaultCluster}"
: "${ROCKETMQ_QUEUES:=4}"

# mqadmin lives next to this image's WORKDIR; resolve it instead of pinning the
# distribution version.
resolve_mqadmin() {
  if [ -x ./mqadmin ]; then
    echo ./mqadmin
    return 0
  fi
  for candidate in /home/rocketmq/rocketmq-*/bin/mqadmin; do
    if [ -x "$candidate" ]; then
      echo "$candidate"
      return 0
    fi
  done
  echo "mqadmin not found" >&2
  return 1
}

MQADMIN="$(resolve_mqadmin)"
ADMIN_OPTS="-n $ROCKETMQ_NAMESRV -c $ROCKETMQ_CLUSTER"

wait_for_cluster() {
  attempt=0
  while [ "$attempt" -lt 60 ]; do
    if sh "$MQADMIN" clusterList -n "$ROCKETMQ_NAMESRV" 2>/dev/null | grep -q broker-a; then
      echo "rocketmq broker is up"
      return 0
    fi
    attempt=$((attempt + 1))
    sleep 2
  done
  echo "rocketmq broker did not come up in time" >&2
  return 1
}

wait_for_cluster

update_topic() {
  topic="$1"
  attributes="$2"
  echo "ensuring topic: $topic ${attributes}"
  # shellcheck disable=SC2086
  sh "$MQADMIN" updateTopic $ADMIN_OPTS -t "$topic" -r "$ROCKETMQ_QUEUES" -w "$ROCKETMQ_QUEUES" ${attributes}
}

# ── Ordered topics: chat delivery and read receipts ─────────────────────────
update_topic aim-message-transfer "-a +message.type=FIFO"
update_topic aim-read-receipt-events "-a +message.type=FIFO"

# ── Transactional topics: events published with a local transaction ─────────
update_topic aim-user-events "-a +message.type=TRANSACTION"
update_topic aim-conversation-events "-a +message.type=TRANSACTION"
update_topic aim-attachment-uploaded "-a +message.type=TRANSACTION"

# ── Normal topics ───────────────────────────────────────────────────────────
update_topic aim-presence-events ""
update_topic aim-typing-events ""
update_topic aim-friend-application-events ""
update_topic aim-attachment-parsed ""

# ── Consumer groups ─────────────────────────────────────────────────────────
CONSUMER_GROUPS="
aim-core-delivery
aim-core-presence
aim-core-typing
aim-core-read-receipt
aim-core-conversation-events
aim-core-attachment-parsed
aim-core-friend-application
aim-logic-archive
aim-logic-user-created
aim-logic-bot-webhook
aim-data-parsing-uploaded
"

# Groups that consume FIFO topics must consume in order.
ORDERED_GROUPS="
aim-core-delivery
aim-core-read-receipt
aim-logic-archive
aim-logic-bot-webhook
"

# ── Producer groups ─────────────────────────────────────────────────────────
PRODUCER_GROUPS="
aim-gateway-producer
aim-core-transfer
aim-logic-producer
aim-auth-producer
aim-attachment-producer
aim-data-parsing-producer
"

for group in $CONSUMER_GROUPS $PRODUCER_GROUPS; do
  echo "ensuring group: $group"
  # shellcheck disable=SC2086
  sh "$MQADMIN" updateSubGroup $ADMIN_OPTS -g "$group"
done

for group in $ORDERED_GROUPS; do
  echo "ensuring ordered consumption: $group"
  # shellcheck disable=SC2086
  sh "$MQADMIN" updateSubGroup $ADMIN_OPTS -g "$group" -o true
done

echo "rocketmq topics and groups are ready"
