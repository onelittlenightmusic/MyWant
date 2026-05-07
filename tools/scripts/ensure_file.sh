#!/bin/sh
# ensure_file.sh — Download or build a file if it does not already exist.
#
# Reads state from $MYWANT_CURRENT_FILE (JSON).
# Outputs {"current_updates": {...}} to stdout.
#
# Supported methods: curl | docker_wget | docker_build
set -e

FILENAME=$(jq -r '.filename // ""' "$MYWANT_CURRENT_FILE")
DATA_DIR=$(jq -r '.data_dir // "/tmp/ensure-file"' "$MYWANT_CURRENT_FILE")
METHOD=$(jq -r '.method // "curl"' "$MYWANT_CURRENT_FILE")
TARGET="$DATA_DIR/$FILENAME"

# Already done
if [ "$(jq -r '.ensure_file_phase // ""' "$MYWANT_CURRENT_FILE")" = "done" ]; then
  echo '{"current_updates": {"ensure_file_phase": "done", "achieving_percentage": 100}}'
  exit 0
fi

# File already exists on disk
if [ -f "$TARGET" ]; then
  echo "{\"current_updates\": {\"ensure_file_phase\": \"done\", \"ensure_file_path\": \"$TARGET\", \"achieving_percentage\": 100}}"
  exit 0
fi

# Check prerequisites — wait until all prerequisite files exist
PREREQS=$(jq -r '.prerequisites // "[]"' "$MYWANT_CURRENT_FILE")
PREREQ_COUNT=$(echo "$PREREQS" | jq 'length')
if [ "$PREREQ_COUNT" != "0" ]; then
  i=0
  while [ $i -lt "$PREREQ_COUNT" ]; do
    PREREQ=$(echo "$PREREQS" | jq -r ".[$i]")
    if [ ! -f "$DATA_DIR/$PREREQ" ]; then
      echo '{"current_updates": {"ensure_file_phase": "waiting", "achieving_percentage": 5}}'
      exit 0
    fi
    i=$((i + 1))
  done
fi

mkdir -p "$DATA_DIR"
echo "{\"current_updates\": {\"ensure_file_phase\": \"starting\", \"ensure_file_path\": \"$TARGET\", \"achieving_percentage\": 10}}"

case "$METHOD" in
  curl)
    URL=$(jq -r '.url // ""' "$MYWANT_CURRENT_FILE")
    URL=$(eval echo "$URL")   # expand env vars in URL, e.g. ${API_KEY}
    if [ -z "$URL" ]; then
      echo '{"current_updates": {"ensure_file_phase": "failed", "ensure_file_error": "url param required for curl method"}}'
      exit 0
    fi
    if curl -fsSL --user-agent "mywant-ensure-file/2.0" -o "$TARGET" "$URL"; then
      echo '{"current_updates": {"ensure_file_phase": "done", "achieving_percentage": 100}}'
    else
      echo '{"current_updates": {"ensure_file_phase": "failed", "ensure_file_error": "curl download failed"}}'
    fi
    ;;

  docker_wget)
    URL=$(jq -r '.url // ""' "$MYWANT_CURRENT_FILE")
    if [ -z "$URL" ]; then
      echo '{"current_updates": {"ensure_file_phase": "failed", "ensure_file_error": "url param required for docker_wget method"}}'
      exit 0
    fi
    VOLUMES=$(jq -r '.docker_volumes // "[]"' "$MYWANT_CURRENT_FILE")
    if [ "$(echo "$VOLUMES" | jq 'length')" = "0" ]; then
      VOLUMES="[\"$DATA_DIR:/data\"]"
    fi
    VOLUME_ARGS=$(echo "$VOLUMES" | jq -r '.[] | "-v " + .' | tr '\n' ' ')
    # shellcheck disable=SC2086
    if docker run --rm $VOLUME_ARGS alpine sh -c "wget -O /data/$FILENAME '$URL'"; then
      echo '{"current_updates": {"ensure_file_phase": "done", "achieving_percentage": 100}}'
    else
      echo '{"current_updates": {"ensure_file_phase": "failed", "ensure_file_error": "docker_wget failed"}}'
    fi
    ;;

  docker_build)
    IMAGE=$(jq -r '.docker_image // ""' "$MYWANT_CURRENT_FILE")
    if [ -z "$IMAGE" ]; then
      echo '{"current_updates": {"ensure_file_phase": "failed", "ensure_file_error": "docker_image param required for docker_build method"}}'
      exit 0
    fi
    VOLUMES=$(jq -r '.docker_volumes // "[]"' "$MYWANT_CURRENT_FILE")
    if [ "$(echo "$VOLUMES" | jq 'length')" = "0" ]; then
      VOLUMES="[\"$DATA_DIR:/var/opentripplanner\"]"
    fi
    ENV_OBJ=$(jq -r '.docker_env // "{}"' "$MYWANT_CURRENT_FILE")
    CMD_ARGS=$(jq -r '.docker_command_args // "[]"' "$MYWANT_CURRENT_FILE")
    VOLUME_ARGS=$(echo "$VOLUMES" | jq -r '.[] | "-v " + .' | tr '\n' ' ')
    ENV_ARGS=$(echo "$ENV_OBJ" | jq -r 'to_entries[] | "-e " + .key + "=" + .value' | tr '\n' ' ')
    ARGS=$(echo "$CMD_ARGS" | jq -r '.[]' | tr '\n' ' ')
    # shellcheck disable=SC2086
    if docker run --rm $VOLUME_ARGS $ENV_ARGS "$IMAGE" $ARGS; then
      echo '{"current_updates": {"ensure_file_phase": "done", "achieving_percentage": 100}}'
    else
      echo '{"current_updates": {"ensure_file_phase": "failed", "ensure_file_error": "docker_build failed"}}'
    fi
    ;;

  *)
    echo "{\"current_updates\": {\"ensure_file_phase\": \"failed\", \"ensure_file_error\": \"unknown method: $METHOD\"}}"
    ;;
esac
