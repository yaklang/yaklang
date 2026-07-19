#!/usr/bin/env bash
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
dataset="$root/benchmarks/harbor/datasets/yak-agent-v1"

if ! docker info >/dev/null 2>&1; then
  echo "Docker daemon is not available" >&2
  exit 1
fi

for task in "$dataset"/*; do
  [[ -f "$task/task.toml" ]] || continue
  name="$(basename "$task")"
  image="yak-harbor-${name}:local"
  docker build -q -t "$image" "$task/environment" >/dev/null

  for attempt in 1 2 3; do
    container="$(docker run -d "$image")"
    trap 'docker rm -f "$container" >/dev/null 2>&1 || true' EXIT
    docker exec "$container" mkdir -p /logs/verifier
    docker cp "$task/tests/." "$container:/tests"
    docker exec "$container" bash /tests/test.sh >/dev/null
    noop_reward="$(docker exec "$container" python -c \
      'import json; print(json.load(open("/logs/verifier/reward.json"))["reward"])')"
    docker rm -f "$container" >/dev/null
    trap - EXIT
    [[ "$noop_reward" == "0.0" ]] || {
      echo "$name noop attempt $attempt returned $noop_reward" >&2
      exit 1
    }

    container="$(docker run -d "$image")"
    trap 'docker rm -f "$container" >/dev/null 2>&1 || true' EXIT
    docker exec "$container" mkdir -p /logs/verifier
    docker cp "$task/tests/." "$container:/tests"
    docker cp "$task/solution/." "$container:/solution"
    docker exec "$container" bash /solution/solve.sh >/dev/null
    docker exec "$container" bash /tests/test.sh >/dev/null
    oracle_reward="$(docker exec "$container" python -c \
      'import json; print(json.load(open("/logs/verifier/reward.json"))["reward"])')"
    docker rm -f "$container" >/dev/null
    trap - EXIT
    [[ "$oracle_reward" == "1.0" ]] || {
      echo "$name oracle attempt $attempt returned $oracle_reward" >&2
      exit 1
    }
  done
  echo "$name: noop 0.0 x3, oracle 1.0 x3"
done

