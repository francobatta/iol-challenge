#!/usr/bin/env bash
# Load-tests the cluster deploy/cluster-up.sh created, and checks that it coped.
#
#   deploy/loadtest.sh
#   USERS=50000 deploy/loadtest.sh
#
# It works as an app would, through the API: it creates an app and a list, imports USERS
# users with an endpoint on each of the four providers as one CSV, and sends the list one
# notification. That is 4 x USERS deliveries, a backlog KEDA has to grow every worker
# pool for.
#
# It then watches until everything is delivered and the pools are back to one pod, and
# asks each part of the system whether the job is really done:
#
#   API         the job is dispatched, with every delivery queued
#   Postgres    the same in the jobs table, and no fan-out left over
#   RabbitMQ    nothing waiting or dead; no alarms
#   Metrics     Prometheus counted every delivery as sent and none as failed; Grafana is up
#   Kubernetes  every pool grew past one pod and shrank again; nothing restarted
#
# Every check prints PASS or FAIL, and the script exits 1 if any failed.

. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# Users to import. The default fills each pool's queue fifty times past the 400
# deliveries KEDA allows per pod.
USERS=${USERS:-20000}
# Seconds to wait for the deliveries, and then again for the pools to shrink.
TIMEOUT=${TIMEOUT:-900}

EXPECTED=$((USERS * 4))

need kubectl curl awk sed
cluster_exists || die "there is no cluster; run deploy/cluster-up.sh first"

# --- Talking to the system -------------------------------------------------------------

# http METHOD URL [CURL ARGUMENT...] makes a request and sets HTTP_STATUS and HTTP_BODY.
http() {
  local method=$1 url=$2 out
  shift 2
  out=$(curl -sS -X "$method" "$url" -w '\n%{http_code}' ${1+"$@"}) || die "$method $url: no answer"
  HTTP_STATUS=${out##*$'\n'}
  HTTP_BODY=${out%$'\n'*}
  HTTP_BODY=${HTTP_BODY%$'\n'} # the API ends its answers with a newline
}

# api METHOD PATH [CURL ARGUMENT...] is http against the API, as the app under test.
api() {
  local method=$1 path=$2
  shift 2
  http "$method" "$API_URL$path" -H "Authorization: Bearer $TOKEN" ${1+"$@"}
}

# want STATUS WHAT stops the test unless the last request was answered with STATUS.
want() {
  [ "$HTTP_STATUS" = "$1" ] || die "$2: got HTTP $HTTP_STATUS, want $1: $HTTP_BODY"
}

# field NAME prints a string or number field of the JSON object in HTTP_BODY.
field() {
  printf '%s' "$HTTP_BODY" | sed -n 's/.*"'"$1"'":"\{0,1\}\([^",}]*\).*/\1/p'
}

# sql QUERY prints the answer of Postgres, one row per line.
sql() { k exec db-0 -- psql -U audience -d audience -AtXc "$1"; }

# queues prints "name messages" for every queue of RabbitMQ.
queues() { k exec rabbitmq-0 -- rabbitmqctl -q --no-table-headers list_queues name messages; }

# messages PATTERN prints how many messages wait in the queues whose name matches.
messages() { queues | awk -v pattern="$1" '$1 ~ pattern { n += $2 } END { print n + 0 }'; }

# prom QUERY prints the one number a PromQL query comes to, or 0 if it matches nothing.
prom() {
  local out
  out=$(curl -sS -G "$PROMETHEUS_URL/api/v1/query" --data-urlencode "query=$1") || die "Prometheus: no answer"
  out=$(printf '%s' "$out" | sed -n 's/.*"value":\[[^,]*,"\([^"]*\)"\].*/\1/p')
  printf '%s' "${out:-0}"
}

# counted METRIC SELECTOR prints the total of a counter for the app. Workers come and go
# during the test and each counts for itself, so the total is the sum of the highest
# value each of them ever reported.
counted() { prom "sum(max_over_time($1{app_id=\"$APP_ID\"$2}[1h]))"; }

# pools prints "provider=ready/wanted" for every worker pool.
pools() {
  k get deployments -l app=worker \
    -o jsonpath='{range .items[*]}{.metadata.labels.provider}={.status.readyReplicas}/{.spec.replicas}{" "}{end}'
}

# restarts prints how often the containers of the pods now running have been restarted.
restarts() {
  k get pods -o jsonpath='{range .items[*]}{range .status.containerStatuses[*]}{.restartCount}{"\n"}{end}{end}' |
    awk '{ n += $1 } END { print n + 0 }'
}

# --- Checks ----------------------------------------------------------------------------

failures=0

# check WHAT GOT OPERATOR WANT prints PASS or FAIL for one expectation. The operator is
# one of test's: = for text, -eq -ge -gt for numbers.
check() {
  if [ "$2" "$3" "$4" ] 2>/dev/null; then
    printf '  PASS  %s (%s)\n' "$1" "$2"
  else
    printf '  FAIL  %s: got %s, want %s %s\n' "$1" "${2:-nothing}" "$3" "$4"
    failures=$((failures + 1))
  fi
}

# --- Set up ----------------------------------------------------------------------------

log "Checking the cluster"
k get deployment api >/dev/null || die "the system is not deployed; run deploy/cluster-up.sh"
k wait --for=condition=Available deployment --all --timeout=60s >/dev/null ||
  die "not every Deployment is available; see: kubectl -n $NAMESPACE get pods"
restarts_before=$(restarts)
for provider in $PROVIDERS; do
  eval "peak_$provider=0"
done

log "Creating an app and a list"
TOKEN=
http POST "$API_URL/v1/apps" -H "X-Admin-Key: $ADMIN_KEY" -d "{\"name\": \"loadtest-$(date +%s)\"}"
want 201 "creating the app"
APP_ID=$(field app_id)
TOKEN=$(field token)
api POST /v1/lists -d '{"name": "everyone", "description": "The audience of the load test"}'
want 201 "creating the list"
LIST_ID=$(field list_id)
echo "  app $APP_ID, list $LIST_ID"

log "Importing $USERS users with $EXPECTED endpoints"
# audience prints the CSV to import, one row per endpoint: user_id,channel,provider,address.
audience() {
  awk -v users="$USERS" 'BEGIN {
  for (i = 1; i <= users; i++) {
    user = sprintf("user-%07d", i)
    printf "%s,sms,twilio,+54911%07d\n", user, i
    printf "%s,email,mailchimp,%s@example.com\n", user, user
    printf "%s,push,apns,ios-token-%07d\n", user, i
    printf "%s,push,fcm,android-token-%07d\n", user, i
  }
}'
}
started=$(date +%s)
# The file is never written down: it goes from awk straight into the request.
api POST "/v1/users/import?list_id=$LIST_ID" -H 'Content-Type: text/csv' --data-binary @- < <(audience)
want 200 "importing the users"
echo "  $HTTP_BODY in $(( $(date +%s) - started ))s"
imported_users=$(field users)
imported_endpoints=$(field endpoints)

log "Sending one notification to the list"
api POST /v1/notifications -H "Idempotency-Key: loadtest-$APP_ID" \
  -d "{\"list_id\": \"$LIST_ID\", \"priority\": \"normal\", \"content\": {\"title\": \"Load test\", \"body\": \"Hello from the load test\"}}"
want 202 "sending the notification"
JOB_ID=$(field job_id)
echo "  job $JOB_ID"

# --- Watch -----------------------------------------------------------------------------

log "Watching the job, the queues and the worker pools (every 5s, for up to ${TIMEOUT}s)"
printf '  %6s  %-11s %8s %8s %8s   %s\n' time job queued waiting sent "pods ready/wanted"
started=$(date +%s)
delivered_at=
calm=no
while :; do
  elapsed=$(( $(date +%s) - started ))
  api GET "/v1/notifications/$JOB_ID"
  status=$(field status)
  queued=$(field queued)
  waiting=$(messages "^notify[.]send[.].*[.]$APP_ID\$")
  sent=$(counted notify_deliveries_total ',outcome="sent"')
  state=$(pools)

  calm=yes
  for pool in $state; do
    provider=${pool%%=*}
    ready=${pool#*=}
    wanted=${ready#*/}
    ready=${ready%/*}
    ready=${ready:-0}
    eval "peak=\$peak_$provider"
    if [ "$ready" -gt "$peak" ]; then eval "peak_$provider=$ready"; fi
    if [ "$ready" != 1 ] || [ "$wanted" != 1 ]; then calm=no; fi
  done
  printf '  %5ss  %-11s %8s %8s %8s   %s\n' "$elapsed" "$status" "$queued" "$waiting" "$sent" "$state"

  if [ -z "$delivered_at" ] && [ "$status" = dispatched ] && [ "$waiting" -eq 0 ] && [ "${sent%.*}" -ge "$EXPECTED" ]; then
    delivered_at=$elapsed
    echo "  -- everything delivered after ${delivered_at}s; waiting for the pools to shrink"
  fi
  if [ -n "$delivered_at" ] && [ "$calm" = yes ]; then break; fi
  if [ -z "$delivered_at" ] && [ "$elapsed" -ge "$TIMEOUT" ]; then
    echo "  -- not delivered after ${TIMEOUT}s; giving up"
    break
  fi
  if [ -n "$delivered_at" ] && [ $((elapsed - delivered_at)) -ge "$TIMEOUT" ]; then
    echo "  -- the pools did not shrink within ${TIMEOUT}s; giving up"
    break
  fi
  sleep 5
done

# --- Assess ----------------------------------------------------------------------------

log "API"
api GET "/v1/notifications/$JOB_ID"
check "job status" "$(field status)" = dispatched
check "deliveries queued by the job" "$(field queued)" -eq "$EXPECTED"
check "users created by the import" "$imported_users" -eq "$USERS"
check "endpoints created by the import" "$imported_endpoints" -eq "$EXPECTED"

log "Postgres"
check "jobs row" "$(sql "SELECT status || ' ' || queued FROM jobs WHERE job_id = '$JOB_ID'")" = "dispatched $EXPECTED"
check "fan-outs left for the job" "$(sql "SELECT count(*) FROM fanouts WHERE job_id = '$JOB_ID'")" -eq 0
check "deliveries counted against the quota today" "$(sql "SELECT COALESCE(sum(queued), 0) FROM usage_daily WHERE app_id = '$APP_ID'")" -eq "$EXPECTED"
check "members of the list" "$(sql "SELECT count(*) FROM list_members WHERE list_id = '$LIST_ID'")" -eq "$USERS"

log "RabbitMQ"
check "send queues of the app" "$(queues | grep -c "^notify[.]send[.].*[.]$APP_ID" || true)" -eq 4
check "deliveries waiting in them" "$(messages "^notify[.]send[.].*[.]$APP_ID\$")" -eq 0
check "dead deliveries" "$(messages '^notify[.]dead$')" -eq 0
if k exec rabbitmq-0 -- rabbitmq-diagnostics -q check_local_alarms >/dev/null 2>&1; then alarms=none; else alarms=raised; fi
check "resource alarms" "$alarms" = none

log "Metrics"
api GET "/v1/metrics?range=15m"
check "GET /v1/metrics" "$HTTP_STATUS" = 200
check "deliveries queued, by the api" "$(counted notify_fanout_deliveries_total '')" -eq "$EXPECTED"
# At least once: a worker stopped in the middle of a send may send it again.
check "deliveries sent, by the workers" "$(counted notify_deliveries_total ',outcome="sent"')" -ge "$EXPECTED"
check "deliveries failed, by the workers" "$(counted notify_deliveries_total ',outcome="failed"')" -eq 0
# Targets that are up, rather than none that are down: the pods KEDA has just taken away
# stay listed as down for a moment.
check "api pods scraped" "$(prom 'count(up{job="api"} == 1)')" -ge 1
check "worker pods scraped" "$(prom 'count(up{job="worker"} == 1)')" -ge 4
check "RabbitMQ scraped" "$(prom 'count(up{job="rabbitmq"} == 1)')" -eq 1
check "requests counted, by the api" "$(prom 'sum(notify_http_requests_total)')" -gt 0
grafana=$(curl -sS "$GRAFANA_URL/api/health" | sed -n 's/.*"database": *"\([^"]*\)".*/\1/p') || die "Grafana: no answer"
check "Grafana" "${grafana:-no answer}" = ok

log "Kubernetes"
for provider in $PROVIDERS; do
  eval "peak=\$peak_$provider"
  check "most pods of worker-$provider at once" "$peak" -gt 1
done
for pool in $(pools); do
  check "pods of worker-${pool%%=*} now, ready/wanted" "${pool#*=}" = 1/1
done
ready_scalers=$(k get scaledobjects -o jsonpath='{range .items[*]}{.status.conditions[?(@.type=="Ready")].status}{"\n"}{end}' | grep -c True || true)
check "ScaledObjects that are ready" "$ready_scalers" -eq 4
lagging=$(k get deployments -o jsonpath='{range .items[*]}{.metadata.name}={.status.readyReplicas}/{.spec.replicas}{"\n"}{end}' |
  awk -F'[=/]' '$2 != $3 { printf "%s ", $1 }')
check "Deployments without all their pods ready" "${lagging:-none}" = none
check "container restarts during the test" "$(( $(restarts) - restarts_before ))" -eq 0

echo
echo "  What the autoscaler did:"
k get events --field-selector reason=SuccessfulRescale --sort-by=.lastTimestamp \
  -o custom-columns=TIME:.lastTimestamp,POOL:.involvedObject.name,CHANGE:.message --no-headers |
  tail -n 16 | sed -e 's/(&LabelSelector[^)]*)//' -e 's/^/    /'

echo
if [ "$failures" -gt 0 ]; then
  echo "FAILED: $failures checks did not pass."
  exit 1
fi
echo "PASSED: $EXPECTED deliveries, delivered in ${delivered_at}s, with every worker pool scaled up and back down."
