import { useState, type ReactNode } from "react"
import { keepPreviousData, useQuery } from "@tanstack/react-query"
import { CircleAlertIcon, CircleCheckIcon, CircleXIcon } from "lucide-react"
import { cn } from "cn"

import { ErrorNote, PageHeader } from "@/components/common"
import { TimeSeriesPanel, type LineSpec } from "@/components/TimeSeriesPanel"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableFooter, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api, ApiError, type ProviderDeliveries, type Series, type Snapshot } from "@/lib/api"
import { formatBytes, formatCount, formatCPU, formatExact, formatRate, formatSeconds } from "@/lib/format"

const ranges = ["5m", "15m", "1h", "6h", "24h"]

/** How often the dashboard asks again. Prometheus collects every 5 seconds, so sooner shows nothing new. */
const refreshMillis = 5000

// A provider has the same color in every chart, and the outcome of a delivery wears a
// status color. The values are in index.css.
const providerLines: LineSpec[] = [
  { key: "twilio", label: "Twilio", color: "var(--series-twilio)" },
  { key: "mailchimp", label: "Mailchimp", color: "var(--series-mailchimp)" },
  { key: "apns", label: "APNs", color: "var(--series-apns)" },
  { key: "fcm", label: "FCM", color: "var(--series-fcm)" },
]
const outcomeLines: LineSpec[] = [
  { key: "sent", label: "Sent", color: "var(--status-good)" },
  { key: "retried", label: "Retried", color: "var(--status-warning)" },
  { key: "failed", label: "Failed", color: "var(--status-critical)" },
  { key: "undecodable", label: "Undecodable", color: "var(--status-serious)" },
]
const fanoutLines: LineSpec[] = [
  { key: "pages", label: "Pages fanned out", color: "var(--series-twilio)" },
  { key: "rejected", label: "Pages put off, queue full", color: "var(--status-critical)" },
]
const providerLabels = Object.fromEntries(providerLines.map((line) => [line.key, line]))

export default function Dashboard() {
  const [range, setRange] = useState("15m")
  const metrics = useQuery({
    queryKey: ["metrics", range],
    queryFn: () => api.metrics(range),
    refetchInterval: refreshMillis,
    placeholderData: keepPreviousData, // changing the range keeps the old charts until the new ones arrive
  })

  return (
    <>
      <PageHeader
        title="Dashboard"
        description="What became of your app's deliveries, and how the system that makes them is doing. Refreshes every 5 seconds."
        action={
          <div role="group" aria-label="Time range" className="flex rounded-lg bg-muted p-[3px]">
            {ranges.map((option) => (
              <button
                key={option}
                type="button"
                aria-pressed={option === range}
                onClick={() => setRange(option)}
                className={cn(
                  "rounded-md px-2.5 py-1 text-sm text-muted-foreground outline-none hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50",
                  option === range && "bg-background font-medium text-foreground shadow-sm",
                )}
              >
                {option}
              </button>
            ))}
          </div>
        }
      />
      {metrics.data ? (
        <>
          {metrics.isError && (
            <p role="status" className="rounded-lg border px-3 py-2 text-sm text-muted-foreground">
              The metrics could not be refreshed. These are the last ones read; this page keeps trying.
            </p>
          )}
          <AppSection snapshot={metrics.data} range={range} />
          <SystemSection snapshot={metrics.data} />
        </>
      ) : metrics.isPending ? (
        <div className="grid gap-4 md:grid-cols-2">
          <Skeleton className="h-64" />
          <Skeleton className="h-64" />
        </div>
      ) : (
        <Unavailable error={metrics.error} />
      )}
    </>
  )
}

/** Shown in place of the dashboard when the metrics cannot be read; the rest of the console still works. */
function Unavailable({ error }: { error: unknown }) {
  if (!(error instanceof ApiError && error.status === 503)) {
    return <ErrorNote error={error} />
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>Metrics are unavailable</CardTitle>
        <CardDescription>
          The API cannot reach Prometheus, where delivery metrics are kept. Users, lists and notifications are not affected, and this page
          tries again on its own.
        </CardDescription>
      </CardHeader>
    </Card>
  )
}

function AppSection({ snapshot, range }: { snapshot: Snapshot; range: string }) {
  const { providers } = snapshot.app
  const total = (pick: (provider: ProviderDeliveries) => number) => providers.reduce((sum, provider) => sum + pick(provider), 0)
  const columns: { label: string; hint: string; pick: (provider: ProviderDeliveries) => number }[] = [
    { label: "Queued", hint: `in the last ${range}`, pick: (p) => p.queued },
    { label: "Waiting", hint: "now", pick: (p) => p.ready },
    { label: "With a worker", hint: "now", pick: (p) => p.unacked },
    { label: "Sent", hint: `in the last ${range}`, pick: (p) => p.sent },
    { label: "Retried", hint: `in the last ${range}`, pick: (p) => p.retried },
    { label: "Failed", hint: `in the last ${range}`, pick: (p) => p.failed },
    { label: "In transit", hint: "now", pick: (p) => p.in_transit },
  ]

  return (
    <Section title="Your app" description="Only your app's deliveries, per provider.">
      <Card className="md:col-span-2">
        <CardHeader>
          <CardTitle>Delivery pipeline</CardTitle>
          <CardDescription>
            Left to right is the path of a delivery: queued by the API, waiting for a worker, held by one, then sent, retried or failed.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Provider</TableHead>
                {columns.map((column) => (
                  <TableHead key={column.label} className="text-right">
                    {column.label}
                    <span className="block text-xs font-normal text-muted-foreground">{column.hint}</span>
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {providers.map((provider) => (
                <TableRow key={provider.provider}>
                  <TableCell>
                    <span className="flex items-center gap-2 font-medium">
                      <span className="size-2 rounded-full" style={{ background: providerLabels[provider.provider]?.color }} aria-hidden />
                      {providerLabels[provider.provider]?.label ?? provider.provider}
                      {provider.breaker_open && (
                        <Badge variant="destructive" title="Workers have paused this app on this provider after repeated failures">
                          <CircleAlertIcon aria-hidden />
                          Paused
                        </Badge>
                      )}
                    </span>
                  </TableCell>
                  {columns.map((column) => (
                    <TableCell key={column.label} className="text-right" title={formatExact(column.pick(provider))}>
                      {formatCount(column.pick(provider))}
                    </TableCell>
                  ))}
                </TableRow>
              ))}
            </TableBody>
            <TableFooter>
              <TableRow>
                <TableCell>All providers</TableCell>
                {columns.map((column) => (
                  <TableCell key={column.label} className="text-right" title={formatExact(total(column.pick))}>
                    {formatCount(total(column.pick))}
                  </TableCell>
                ))}
              </TableRow>
            </TableFooter>
          </Table>
        </CardContent>
      </Card>
      <TimeSeriesPanel
        title="Deliveries handled"
        description="Per second, by what became of them."
        series={snapshot.app.outcome_rate}
        by="outcome"
        lines={outcomeLines}
        format={formatRate}
      />
      <TimeSeriesPanel
        title="Deliveries queued"
        description="Per second, as the API fans jobs out."
        series={snapshot.app.queued_rate}
        by="provider"
        lines={providerLines}
        format={formatRate}
      />
      <div className="md:col-span-2">
        <TimeSeriesPanel
          title="Backlog"
          description="Deliveries in your app's queues, waiting or with a worker."
          series={snapshot.app.backlog}
          by="provider"
          lines={providerLines}
          format={formatCount}
        />
      </div>
    </Section>
  )
}

const quantiles = [
  { key: "latency_p50", label: "p50" },
  { key: "latency_p95", label: "p95" },
  { key: "latency_p99", label: "p99" },
] as const

function SystemSection({ snapshot }: { snapshot: Snapshot }) {
  const { system } = snapshot
  const [quantile, setQuantile] = useState<(typeof quantiles)[number]>(quantiles[1])
  const labelled = (series: Series[], kind: string) => series.map((one) => ({ ...one, labels: { kind } }))
  const deepest = Math.max(1, ...system.queues.map((queue) => queue.value))

  return (
    <Section title="System" description="Shared by every app: the processes, the worker pools and the queues deliveries wait in to be retried.">
      <Card>
        <CardHeader>
          <CardTitle>Processes</CardTitle>
          <CardDescription>What Prometheus scrapes, and whether it answered the last time.</CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Process</TableHead>
                <TableHead>State</TableHead>
                <TableHead className="text-right">Goroutines</TableHead>
                <TableHead className="text-right">Memory</TableHead>
                <TableHead className="text-right">CPU</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {system.targets.map((target) => {
                const isGo = target.goroutines > 0 // RabbitMQ reports none of these about itself
                return (
                  <TableRow key={`${target.job}/${target.instance}`}>
                    <TableCell>
                      <span className="font-medium">{target.job}</span>
                      <span className="block text-xs text-muted-foreground">{target.instance}</span>
                    </TableCell>
                    <TableCell>
                      <span className="flex items-center gap-1.5">
                        {target.up ? (
                          <CircleCheckIcon className="size-4 text-(--status-good)" aria-hidden />
                        ) : (
                          <CircleXIcon className="size-4 text-(--status-critical)" aria-hidden />
                        )}
                        {target.up ? "Up" : "Down"}
                      </span>
                    </TableCell>
                    <TableCell className="text-right">{isGo ? formatExact(target.goroutines) : "–"}</TableCell>
                    <TableCell className="text-right">{isGo ? formatBytes(target.memory_bytes) : "–"}</TableCell>
                    <TableCell className="text-right">{isGo ? formatCPU(target.cpu) : "–"}</TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      <div className="flex flex-col gap-4">
        <Card>
          <CardHeader>
            <CardTitle>Worker pools</CardTitle>
            <CardDescription>One pool per provider.</CardDescription>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Provider</TableHead>
                  <TableHead className="text-right">Workers</TableHead>
                  <TableHead className="text-right">Sends in progress</TableHead>
                  <TableHead className="text-right">App queues consumed</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {system.pools.map((pool) => (
                  <TableRow key={pool.provider}>
                    <TableCell className="font-medium">{providerLabels[pool.provider]?.label ?? pool.provider}</TableCell>
                    <TableCell className="text-right">{formatExact(pool.workers)}</TableCell>
                    <TableCell className="text-right">{formatExact(pool.sends_in_flight)}</TableCell>
                    <TableCell className="text-right">{formatExact(pool.queues_subscribed)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Retry and dead-letter queues</CardTitle>
            <CardDescription>A failed delivery waits in each tier in turn; after the last it is dead.</CardDescription>
          </CardHeader>
          <CardContent>
            {system.queues.length === 0 ? (
              <p className="text-sm text-muted-foreground">No queues reported yet.</p>
            ) : (
              <ul className="flex flex-col gap-2">
                {[...system.queues].sort(byRetryOrder).map((queue) => (
                  <li key={queue.labels.queue} className="grid grid-cols-[7rem_1fr_3rem] items-center gap-3 text-sm">
                    <span>{queueLabel(queue.labels.queue)}</span>
                    <span className="h-2 overflow-hidden rounded-full bg-muted" aria-hidden>
                      <span
                        className="block h-full rounded-full bg-foreground/70"
                        style={{ width: `${(queue.value / deepest) * 100}%` }}
                      />
                    </span>
                    <span className="text-right" title={formatExact(queue.value)}>
                      {formatCount(queue.value)}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>
      </div>
      <div className="relative">
        <TimeSeriesPanel
          title={`Provider latency, ${quantile.label}`}
          description="How long a request to the provider takes."
          series={system[quantile.key]}
          by="provider"
          lines={providerLines}
          format={formatSeconds}
        />
        <div role="group" aria-label="Percentile" className="absolute top-3 right-3 flex rounded-lg bg-muted p-[3px]">
          {quantiles.map((option) => (
            <button
              key={option.key}
              type="button"
              aria-pressed={option === quantile}
              onClick={() => setQuantile(option)}
              className={cn(
                "rounded-md px-2 py-0.5 text-xs text-muted-foreground outline-none hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50",
                option === quantile && "bg-background font-medium text-foreground shadow-sm",
              )}
            >
              {option.label}
            </button>
          ))}
        </div>
      </div>
      <TimeSeriesPanel
        title="Sends in progress"
        description="Requests to each provider under way."
        series={system.sends_in_flight}
        by="provider"
        lines={providerLines}
        format={formatCount}
      />
      <div className="md:col-span-2">
        <TimeSeriesPanel
          title="Fan-out"
          description="Pages of users the API turns into deliveries per second, and pages it had to put off because a queue was full."
          series={[...labelled(system.fanout_pages_rate, "pages"), ...labelled(system.fanout_rejected_rate, "rejected")]}
          by="kind"
          lines={fanoutLines}
          format={formatRate}
        />
      </div>
    </Section>
  )
}

function Section(props: { title: string; description: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-base font-semibold">{props.title}</h2>
        <p className="text-sm text-muted-foreground">{props.description}</p>
      </div>
      <div className="grid gap-4 md:grid-cols-2">{props.children}</div>
    </section>
  )
}

// The retry tiers in the order a delivery goes through them, then the dead-letter queue.
const retryOrder = ["notify.retry.5s", "notify.retry.30s", "notify.retry.2m", "notify.retry.10m", "notify.dead"]

function byRetryOrder(a: { labels: Record<string, string> }, b: { labels: Record<string, string> }) {
  return retryOrder.indexOf(a.labels.queue) - retryOrder.indexOf(b.labels.queue)
}

function queueLabel(queue: string) {
  if (queue === "notify.dead") {
    return "Dead"
  }
  return `Retry in ${queue.replace("notify.retry.", "")}`
}
