# Shared by the scripts next to it; sourced, not run.
#
# The scripts run in Bash 3.2 or later on Linux, macOS and Windows (Git Bash), and need
# only docker, kubectl and curl. To keep that true:
#
#   - Every path handed to docker, kubectl or curl is relative to the directory the
#     scripts change into, notification-system/. On Windows those programs are not part of
#     Git Bash and do not understand its /c/... paths.
#   - Nothing is written to /dev/null or a temporary file by those programs, for the same
#     reason: data goes through pipes.
#   - No associative arrays, mapfile or jq.

set -euo pipefail

# Git Bash rewrites arguments that look like Unix paths before it starts a Windows program,
# which mangles the paths meant for the inside of a container (/etc/rancher/..., /mockprovider).
export MSYS_NO_PATHCONV=1
export MSYS2_ARG_CONV_EXCL='*'

cd "$(dirname "${BASH_SOURCE[0]}")/.."

# The cluster is one container of this name, with one volume of the same name.
CLUSTER=${CLUSTER:-notify-k3s}
NAMESPACE=notifications
PROVIDERS="twilio mailchimp apns fcm"

K3S_IMAGE=rancher/k3s:v1.37.1-k3s1
KEDA_VERSION=2.21.0

# Where the cluster is reached from the host. The ports differ from the ones compose
# publishes, so that both can run at once. See k8s/overlays/k3s/nodeports.yaml.
KUBE_API_PORT=16443
API_PORT=18080
WEB_PORT=13000
RABBITMQ_PORT=25672
PROMETHEUS_PORT=19090
GRAFANA_PORT=13001
API_URL=http://localhost:$API_PORT
PROMETHEUS_URL=http://localhost:$PROMETHEUS_PORT
GRAFANA_URL=http://localhost:$GRAFANA_PORT

# The placeholder of k8s/overlays/k3s/kustomization.yaml.
ADMIN_KEY=dev-admin-key

# The kubeconfig of the cluster is kept here, so that the scripts never touch ~/.kube.
export KUBECONFIG=deploy/.kube/config

log() { printf '\n==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

need() {
  local tool
  for tool in "$@"; do
    command -v "$tool" >/dev/null 2>&1 || die "$tool is not installed, or not on the PATH"
  done
}

# kubectl in the namespace of the system. Windows programs end lines with \r\n at times.
k() { kubectl -n "$NAMESPACE" "$@" | tr -d '\r'; }

cluster_exists() { [ -n "$(docker ps -aq --filter "name=^${CLUSTER}\$")" ]; }

# retry SECONDS COMMAND... runs the command every 2 seconds until it succeeds.
retry() {
  local deadline=$(( $(date +%s) + $1 ))
  shift
  until "$@" >/dev/null 2>&1; do
    [ "$(date +%s)" -lt "$deadline" ] || return 1
    sleep 2
  done
}
