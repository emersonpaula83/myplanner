#!/bin/bash
MIN_TOTAL=70.0
EXCLUDE="repository"

TOTAL=$(go test ./internal/... -coverprofile=coverage.out 2>/dev/null \
  | grep -v "/$EXCLUDE" \
  | grep "coverage:" \
  | awk '{sum+=$2; n++} END {printf "%.1f", sum/n}')

echo "Coverage: ${TOTAL}%"
if (( $(echo "$TOTAL < $MIN_TOTAL" | bc -l) )); then
  echo "FAIL: coverage ${TOTAL}% < minimum ${MIN_TOTAL}%"
  exit 1
fi
echo "PASS"
