#!/usr/bin/env bash
# Removes the cluster deploy/cluster-up.sh created, with everything stored in it.
#
#   deploy/cluster-down.sh

. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

need docker

if cluster_exists; then
  log "Removing cluster $CLUSTER"
  docker rm -f "$CLUSTER" >/dev/null
else
  log "There is no cluster $CLUSTER"
fi
# The state of k3s, the volumes of Postgres and RabbitMQ included.
docker volume rm -f "$CLUSTER" >/dev/null
rm -rf deploy/.kube
