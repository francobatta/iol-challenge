#!/usr/bin/env bash
# Creates a single-node Kubernetes cluster and runs the whole notification system on it.
#
#   deploy/cluster-up.sh
#
# The cluster is k3s (https://k3s.io) in one Docker container, with KEDA and nothing else
# added. Running the script again rebuilds the images and applies the manifests to the
# cluster that is already there. deploy/cluster-down.sh removes it.
#
# Needs docker, kubectl and curl. It leaves ~/.kube alone: the kubeconfig is written to
# deploy/.kube/config, and
#
#   export KUBECONFIG=$PWD/deploy/.kube/config
#
# points kubectl at the cluster.

. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

need docker kubectl curl
docker info >/dev/null 2>&1 || die "docker is not running"

# Starts the k3s container and waits until its node is ready.
start_cluster() {
  if cluster_exists; then
    log "Cluster $CLUSTER exists; starting it if it is stopped"
    docker start "$CLUSTER" >/dev/null
  else
    log "Creating cluster $CLUSTER ($K3S_IMAGE)"
    # k3s runs its own containers inside this one, which takes a privileged container.
    # Traefik is left out: the node ports published below are all the ingress needed.
    docker run -d --name "$CLUSTER" --hostname "$CLUSTER" \
      --privileged --restart unless-stopped \
      --tmpfs /run --tmpfs /var/run \
      --ulimit nofile=65535:65535 \
      -v "$CLUSTER:/var/lib/rancher/k3s" \
      -p "127.0.0.1:$KUBE_API_PORT:6443" \
      -p "127.0.0.1:$API_PORT:30080" \
      -p "127.0.0.1:$WEB_PORT:30000" \
      -p "127.0.0.1:$RABBITMQ_PORT:31672" \
      -p "127.0.0.1:$PROMETHEUS_PORT:30090" \
      -p "127.0.0.1:$GRAFANA_PORT:30030" \
      "$K3S_IMAGE" server --disable=traefik >/dev/null
  fi

  # k3s writes a kubeconfig for use inside the container; from the host its API server
  # is on the published port.
  retry 120 docker exec "$CLUSTER" test -s /etc/rancher/k3s/k3s.yaml ||
    die "k3s did not start; see: docker logs $CLUSTER"
  mkdir -p deploy/.kube
  docker exec "$CLUSTER" cat /etc/rancher/k3s/k3s.yaml |
    tr -d '\r' |
    sed "s#https://127.0.0.1:6443#https://127.0.0.1:$KUBE_API_PORT#" >"$KUBECONFIG"

  log "Waiting for the node"
  retry 180 kubectl get nodes || die "the Kubernetes API did not answer"
  # The node registers a moment after the API answers.
  retry 60 kubectl get "node/$CLUSTER" || die "the node did not register"
  kubectl wait --for=condition=Ready "node/$CLUSTER" --timeout=180s
}

# Builds the three images and copies them into the node, which pulls nothing of ours.
load_images() {
  log "Building the images"
  docker build -q -t notification-system/api:k3s -f api/Dockerfile . >/dev/null
  docker build -q -t notification-system/worker:k3s -f worker/Dockerfile . >/dev/null
  docker build -q -t notification-system/web:k3s web >/dev/null

  log "Loading the images into the node"
  docker save notification-system/api:k3s notification-system/worker:k3s notification-system/web:k3s |
    docker exec -i "$CLUSTER" ctr -n k8s.io images import - >/dev/null
}

install_keda() {
  log "Installing KEDA $KEDA_VERSION"
  # Server-side: its CRDs are too large for the annotation a client-side apply adds.
  kubectl apply --server-side --force-conflicts \
    -f "https://github.com/kedacore/keda/releases/download/v$KEDA_VERSION/keda-$KEDA_VERSION.yaml" >/dev/null
  # ScaledObjects are checked by KEDA's admission webhook, so it has to be up before any
  # is applied.
  local deployment
  for deployment in $(kubectl -n keda get deployments -o name | tr -d '\r'); do
    kubectl -n keda rollout status "$deployment" --timeout=300s
  done
}

# render DIRECTORY prints the manifests of a kustomization. The load restrictor lets it
# read api/db/schema.sql, which is outside deploy/k8s.
render() { kubectl kustomize --load-restrictor LoadRestrictionsNone "$1"; }

apply_system() {
  local redeploy=no deployment
  if k get deployment api >/dev/null 2>&1; then redeploy=yes; fi

  # Postgres and RabbitMQ go first. The api and the workers exit when they cannot reach
  # them, so started together they would be restarted until they could. What is in
  # deploy/k8s/infra carries the label tier=infra.
  log "Applying deploy/k8s/overlays/k3s: Postgres, RabbitMQ, the mock provider and the monitoring"
  kubectl apply -f deploy/k8s/base/namespace.yaml
  render deploy/k8s/overlays/k3s | kubectl apply -l tier=infra -f -
  k rollout status statefulset/db --timeout=300s
  k rollout status statefulset/rabbitmq --timeout=300s

  log "Applying deploy/k8s/overlays/k3s: everything"
  render deploy/k8s/overlays/k3s | kubectl apply -f -

  if [ "$redeploy" = yes ]; then
    # The images keep their tag when rebuilt, so the pods have to be told to start over.
    log "Restarting the pods on the rebuilt images"
    k rollout restart deployment/api deployment/web deployment/mockprovider >/dev/null
    k rollout restart deployment -l app=worker >/dev/null
  fi

  log "Waiting for the system"
  for deployment in $(k get deployments -o name); do
    k rollout status "$deployment" --timeout=300s
  done
  k wait --for=condition=Ready scaledobject --all --timeout=120s
}

start_cluster
load_images
install_keda
apply_system

log "The system is up"
k get pods
cat <<EOF

  API                  $API_URL   (admin key: $ADMIN_KEY)
  Console              http://localhost:$WEB_PORT
  RabbitMQ management  http://localhost:$RABBITMQ_PORT   (notify / notify)
  Prometheus           $PROMETHEUS_URL
  Grafana              $GRAFANA_URL   (dashboards, traces and logs)

  kubectl              export KUBECONFIG=\$PWD/deploy/.kube/config
  Load test            deploy/loadtest.sh
  Remove the cluster   deploy/cluster-down.sh
EOF
