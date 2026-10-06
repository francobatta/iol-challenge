import { afterEach, describe, expect, it, vi } from "vitest"

import { api, ApiError, checkToken, createApp, setSession } from "@/lib/api"

function answer(status: number, body?: unknown) {
  return new Response(body === undefined ? null : JSON.stringify(body), { status })
}

/** Replaces fetch with one that gives the answers in turn, and returns it to inspect its calls. */
function stubFetch(...answers: Response[]) {
  const fetch = vi.fn<typeof globalThis.fetch>()
  for (const one of answers) {
    fetch.mockResolvedValueOnce(one)
  }
  vi.stubGlobal("fetch", fetch)
  return fetch
}

afterEach(() => {
  setSession(null)
  vi.unstubAllGlobals()
})

describe("calls as the app", () => {
  it("sends the token of the session", async () => {
    const fetch = stubFetch(answer(200, { items: [] }))
    setSession({ token: "tok", onUnauthorized: () => {} })

    await api.users("ana")

    expect(fetch).toHaveBeenCalledWith("/v1/users?after=ana&limit=50", expect.objectContaining({ method: "GET" }))
    expect(fetch.mock.calls[0][1]?.headers).toMatchObject({ Authorization: "Bearer tok" })
  })

  it("escapes IDs that are part of a path", async () => {
    const fetch = stubFetch(answer(200, { user_id: "a/b c" }))
    setSession({ token: "tok", onUnauthorized: () => {} })

    await api.registerUser("a/b c")

    expect(fetch.mock.calls[0][0]).toBe("/v1/users/a%2Fb%20c")
  })

  it("sends the idempotency key with a notification", async () => {
    const fetch = stubFetch(answer(202, { job_id: "j1" }))
    setSession({ token: "tok", onUnauthorized: () => {} })

    await api.sendNotification({ user_ids: ["ana"], priority: "high", content: { title: "", body: "hi" } }, "key-1")

    const request = fetch.mock.calls[0][1]
    expect(request?.headers).toMatchObject({ "Idempotency-Key": "key-1", "Content-Type": "application/json" })
    expect(JSON.parse(request?.body as string)).toEqual({ user_ids: ["ana"], priority: "high", content: { title: "", body: "hi" } })
  })

  it("sends an import file as it is, as CSV", async () => {
    const fetch = stubFetch(answer(200, { rows: 1, users: 1, endpoints: 1 }))
    setSession({ token: "tok", onUnauthorized: () => {} })
    const file = new Blob(["ana,sms,twilio,+5491100000000\n"])

    await expect(api.importUsers(file, "l1")).resolves.toEqual({ rows: 1, users: 1, endpoints: 1 })

    expect(fetch.mock.calls[0][0]).toBe("/v1/users/import?list_id=l1")
    const request = fetch.mock.calls[0][1]
    expect(request?.body).toBe(file)
    expect(request?.headers).toMatchObject({ "Content-Type": "text/csv", Authorization: "Bearer tok" })
  })

  it("returns nothing for an answer without content", async () => {
    stubFetch(answer(204))
    setSession({ token: "tok", onUnauthorized: () => {} })

    await expect(api.deleteUser("ana")).resolves.toBeUndefined()
  })

  it("throws what the API said went wrong", async () => {
    stubFetch(answer(409, { error: { code: "conflict", message: "conflict: name is taken" } }))
    setSession({ token: "tok", onUnauthorized: () => {} })

    const error = await api.createList({ name: "beta", description: "" }).catch((e) => e)

    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({ status: 409, code: "conflict", message: "conflict: name is taken" })
  })

  it("ends the session when the API refuses the token", async () => {
    stubFetch(answer(401, { error: { code: "unauthorized", message: "missing or invalid credentials" } }))
    const onUnauthorized = vi.fn()
    setSession({ token: "old", onUnauthorized })

    await expect(api.users()).rejects.toMatchObject({ status: 401 })
    expect(onUnauthorized).toHaveBeenCalledOnce()
  })

  it("does not call the API without a session", async () => {
    const fetch = stubFetch()

    await expect(api.users()).rejects.toMatchObject({ status: 401 })
    expect(fetch).not.toHaveBeenCalled()
  })

  it("reports an answer that did not come from the API as unreachable", async () => {
    stubFetch(new Response("<html>502 Bad Gateway</html>", { status: 502 }))
    setSession({ token: "tok", onUnauthorized: () => {} })

    await expect(api.users()).rejects.toMatchObject({ status: 502, code: "unreachable" })
  })
})

describe("checkToken", () => {
  it("accepts a token the API accepts", async () => {
    const fetch = stubFetch(answer(200, { items: [] }))

    await expect(checkToken("candidate")).resolves.toBe(true)
    expect(fetch.mock.calls[0][1]?.headers).toMatchObject({ Authorization: "Bearer candidate" })
  })

  it("refuses a token the API refuses, without ending the session", async () => {
    stubFetch(answer(401, { error: { code: "unauthorized", message: "no" } }))
    const onUnauthorized = vi.fn()
    setSession({ token: "current", onUnauthorized })

    await expect(checkToken("candidate")).resolves.toBe(false)
    expect(onUnauthorized).not.toHaveBeenCalled()
  })

  it("throws when the API cannot say", async () => {
    stubFetch(answer(500, { error: { code: "internal", message: "internal error" } }))

    await expect(checkToken("candidate")).rejects.toMatchObject({ status: 500 })
  })
})

describe("createApp", () => {
  it("sends the admin key and no token", async () => {
    const fetch = stubFetch(answer(201, { app_id: "a1", name: "demo", token: "tok" }))

    await expect(createApp("demo", "admin")).resolves.toMatchObject({ token: "tok" })
    const headers = fetch.mock.calls[0][1]?.headers as Record<string, string>
    expect(headers["X-Admin-Key"]).toBe("admin")
    expect(headers.Authorization).toBeUndefined()
  })
})
