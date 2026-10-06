# Notification system

Lets an app register its users and their endpoints, and send them notifications through
Twilio, Mailchimp, APNs and FCM. The providers are mocked.

How it works, and why, is in [DESIGN.md](DESIGN.md).

Every command below is run from this directory, `notification-system/`.

## Run it locally

Needs [Docker](https://docs.docker.com/get-docker/) with Compose v2, and nothing else.

```sh
docker compose up --build -d
```

| What | Where |
|---|---|
| Console | http://localhost:3000 |
| API docs | http://localhost:3000/docs |
| API | http://localhost:8080 |
| Grafana | http://localhost:3001 |
| Prometheus | http://localhost:9090 |
| RabbitMQ management | http://localhost:15672 (`notify` / `notify`) |

To try it, open the console, create an app with the admin key `dev-admin-key`, and sign in
with the token it gives back. Users, lists and notifications are all there.

```sh
docker compose logs -f api   # follow a service
docker compose down -v       # stop it and delete its data
```

## Simulate production

The same images on Kubernetes: a single-node [k3s](https://k3s.io) cluster in one Docker
container, with [KEDA](https://keda.sh) scaling the worker pools.

Needs Docker, [`kubectl`](https://kubernetes.io/docs/tasks/tools/) and `curl`. The scripts
are Bash: on Windows, run them in Git Bash.

```sh
deploy/cluster-up.sh     # create the cluster and run everything on it
deploy/loadtest.sh       # send 80,000 deliveries, watch the workers scale, check the result
deploy/cluster-down.sh   # remove the cluster and its data
```

`cluster-up.sh` takes a few minutes the first time. Run it again after a change to rebuild
and redeploy. `loadtest.sh` prints `PASS` or `FAIL` for every check and exits 1 if any
failed.

The cluster uses ports of its own, so it runs next to compose:

| What | Where |
|---|---|
| Console | http://localhost:13000 |
| API docs | http://localhost:13000/docs |
| API | http://localhost:18080 (admin key `dev-admin-key`) |
| Grafana | http://localhost:13001 |
| Prometheus | http://localhost:19090 |
| RabbitMQ management | http://localhost:25672 (`notify` / `notify`) |

The scripts leave `~/.kube` alone. To use `kubectl` against the cluster:

```sh
export KUBECONFIG=$PWD/deploy/.kube/config
kubectl -n notifications get pods,scaledobjects,hpa
```
