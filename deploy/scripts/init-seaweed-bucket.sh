#!/bin/sh
set -eu

: "${SEAWEED_S3_ENDPOINT:=http://seaweed-s3:8333}"
: "${SEAWEED_BUCKET:=aim-attachments}"
: "${SEAWEED_ACCESS_KEY:?SEAWEED_ACCESS_KEY is required}"
: "${SEAWEED_SECRET_KEY:?SEAWEED_SECRET_KEY is required}"
: "${SEAWEED_INIT_RETRIES:=30}"
: "${SEAWEED_INIT_INTERVAL_SECONDS:=2}"

export AWS_ACCESS_KEY_ID="$SEAWEED_ACCESS_KEY"
export AWS_SECRET_ACCESS_KEY="$SEAWEED_SECRET_KEY"
export AWS_DEFAULT_REGION="${SEAWEED_REGION:-us-east-1}"

for i in $(seq 1 "$SEAWEED_INIT_RETRIES"); do
  if aws --endpoint-url "$SEAWEED_S3_ENDPOINT" s3 ls "s3://$SEAWEED_BUCKET" >/dev/null 2>&1; then
    echo "seaweed bucket exists: $SEAWEED_BUCKET"
    exit 0
  fi

  if aws --endpoint-url "$SEAWEED_S3_ENDPOINT" s3 mb "s3://$SEAWEED_BUCKET"; then
    echo "seaweed bucket created: $SEAWEED_BUCKET"
    exit 0
  fi

  echo "waiting for seaweed s3 ($i/$SEAWEED_INIT_RETRIES)"
  sleep "$SEAWEED_INIT_INTERVAL_SECONDS"
done

echo "failed to initialize seaweed bucket: $SEAWEED_BUCKET" >&2
exit 1
