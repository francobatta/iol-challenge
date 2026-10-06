// The REST API of the notification system, as functions.
//
// Every call but createApp and checkToken is made as the app that is signed in: setSession
// says which token to send and what to do when the API stops accepting it. The types
// mirror the JSON the API writes.

export type User = { user_id: string; created_at: string }
export type Channel = "sms" | "email" | "push"
export type Endpoint = { endpoint_id: string; user_id: string; address: string; channel: Channel; provider: string }
export type List = { list_id: string; name: string; description: string; created_at: string }
export type Member = { user_id: string; added_at: string }
export type Priority = "normal" | "high"
export type JobStatus = "pending" | "dispatching" | "dispatched"
export type Job = { job_id: string; status: JobStatus; priority: Priority; queued: number; created_at: string }
export type Page<T> = { items: T[]; next_after?: string }

export type Sample = { labels: Record<string, string>; value: number }
/** One line of a chart. A point is [Unix seconds, value]. */
export type Series = { labels: Record<string, string>; points: [number, number][] }
export type ProviderDeliveries = {
  provider: string
  queued: number
  sent: number
  retried: number
  failed: number
  undecodable: number
  in_transit: number
  ready: number
  unacked: number
  breaker_open: boolean
}
export type Target = { job: string; instance: string; up: boolean; goroutines: number; memory_bytes: number; cpu: number }
export type WorkerPool = { provider: string; workers: number; sends_in_flight: number; queues_subscribed: number }
export type Snapshot = {
  at: string
  window_seconds: number
  step_seconds: number
  app: { providers: ProviderDeliveries[]; queued_rate: Series[]; outcome_rate: Series[]; backlog: Series[] }
  system: {
    targets: Target[]
    pools: WorkerPool[]
    queues: Sample[]
    latency_p50: Series[]
    latency_p95: Series[]
    latency_p99: Series[]
    sends_in_flight: Series[]
    fanout_pages_rate: Series[]
    fanout_rejected_rate: Series[]
  }
}

/** The providers that deliver on each channel, as the API enforces them. */
export const channelProviders: Record<Channel, string[]> = {
  sms: ["twilio"],
  email: ["mailchimp"],
  push: ["apns", "fcm"],
}

/** The most user IDs one notification, or one request to add members, may name. */
export const maxUserIDs = 1000

/** An answer from the API that is not a success. `code` is the API's own, such as "not_found". */
export class ApiError extends Error {
  readonly status: number
  readonly code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = "ApiError"
    this.status = status
    this.code = code
  }
}

let session: { token: string; onUnauthorized: () => void } | null = null

/** Sets the token later calls are made with. onUnauthorized runs when the API refuses it. */
export function setSession(next: { token: string; onUnauthorized: () => void } | null) {
  session = next
}

type Options = { body?: unknown; headers?: Record<string, string> }

async function send<T>(method: string, path: string, { body, headers }: Options = {}): Promise<T> {
  const response = await fetch(path, {
    method,
    headers: { ...(body === undefined ? {} : { "Content-Type": "application/json" }), ...headers },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (response.status === 204) {
    return undefined as T
  }
  // Anything that is not JSON did not come from the API: the proxy answered for it.
  const answer = await response.json().catch(() => null)
  if (!response.ok) {
    const error = answer?.error
    throw new ApiError(response.status, error?.code ?? "unreachable", error?.message ?? "The API did not answer.")
  }
  return answer as T
}

async function call<T>(method: string, path: string, options: Options = {}): Promise<T> {
  const current = session
  if (!current) {
    throw new ApiError(401, "unauthorized", "Not signed in.")
  }
  try {
    return await send<T>(method, path, { ...options, headers: { ...options.headers, Authorization: `Bearer ${current.token}` } })
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      current.onUnauthorized()
    }
    throw error
  }
}

function query(params: Record<string, string | number | undefined>) {
  const search = new URLSearchParams()
  for (const [name, value] of Object.entries(params)) {
    if (value !== undefined && value !== "") {
      search.set(name, String(value))
    }
  }
  const text = search.toString()
  return text ? `?${text}` : ""
}

const id = encodeURIComponent

/** Reports whether the API accepts the token. It throws only if the API cannot be asked. */
export async function checkToken(token: string): Promise<boolean> {
  try {
    await send("GET", "/v1/users?limit=1", { headers: { Authorization: `Bearer ${token}` } })
    return true
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      return false
    }
    throw error
  }
}

export function createApp(name: string, adminKey: string) {
  return send<{ app_id: string; name: string; token: string }>("POST", "/v1/apps", {
    body: { name },
    headers: { "X-Admin-Key": adminKey },
  })
}

export const pageSize = 50

export const api = {
  users: (after?: string) => call<Page<User>>("GET", `/v1/users${query({ after, limit: pageSize })}`),
  registerUser: (userID: string) => call<User>("PUT", `/v1/users/${id(userID)}`),
  deleteUser: (userID: string) => call<void>("DELETE", `/v1/users/${id(userID)}`),

  endpoints: (userID: string, after?: string) =>
    call<Page<Endpoint>>("GET", `/v1/users/${id(userID)}/endpoints${query({ after, limit: pageSize })}`),
  createEndpoint: (userID: string, endpoint: Pick<Endpoint, "address" | "channel" | "provider">) =>
    call<Endpoint>("POST", `/v1/users/${id(userID)}/endpoints`, { body: endpoint }),
  deleteEndpoint: (endpointID: string) => call<void>("DELETE", `/v1/endpoints/${id(endpointID)}`),

  lists: (after?: string, limit = pageSize) => call<Page<List>>("GET", `/v1/lists${query({ after, limit })}`),
  list: (listID: string) => call<List>("GET", `/v1/lists/${id(listID)}`),
  createList: (list: { name: string; description: string }) => call<List>("POST", "/v1/lists", { body: list }),
  deleteList: (listID: string) => call<void>("DELETE", `/v1/lists/${id(listID)}`),

  members: (listID: string, after?: string) =>
    call<Page<Member>>("GET", `/v1/lists/${id(listID)}/members${query({ after, limit: pageSize })}`),
  addMembers: (listID: string, userIDs: string[]) =>
    call<void>("POST", `/v1/lists/${id(listID)}/members`, { body: { user_ids: userIDs } }),
  removeMember: (listID: string, userID: string) => call<void>("DELETE", `/v1/lists/${id(listID)}/members/${id(userID)}`),

  jobs: (after?: string, limit = pageSize) => call<Page<Job>>("GET", `/v1/notifications${query({ after, limit })}`),
  /** idempotencyKey makes sending the same request twice create one job. */
  sendNotification: (
    notification: { user_ids?: string[]; list_id?: string; priority: Priority; content: { title: string; body: string } },
    idempotencyKey: string,
  ) => call<Job>("POST", "/v1/notifications", { body: notification, headers: { "Idempotency-Key": idempotencyKey } }),

  /** range is a duration the API understands, such as "15m". */
  metrics: (range: string) => call<Snapshot>("GET", `/v1/metrics${query({ range })}`),
}
