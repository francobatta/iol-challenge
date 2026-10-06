import { describe, expect, it } from "vitest"

import { appIDOf } from "@/lib/auth"
import { formatBytes, formatCount, formatRate, formatSeconds } from "@/lib/format"
import { parseIDs } from "@/lib/ids"
import { loadAll } from "@/lib/pages"

describe("appIDOf", () => {
  // A token as the API issues them: {"alg":"HS256","typ":"JWT"}.{"sub":"0199-app"}.signature
  const token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIwMTk5LWFwcCJ9.c2lnbmF0dXJl"

  it("reads the subject of a token", () => {
    expect(appIDOf(token)).toBe("0199-app")
  })

  it.each(["", "not-a-token", "a.b.c", "a.e30.c"])("returns null for %j", (bad) => {
    expect(appIDOf(bad)).toBeNull()
  })
})

describe("parseIDs", () => {
  it.each([
    ["", []],
    ["ana", ["ana"]],
    ["ana, bob,carla", ["ana", "bob", "carla"]],
    ["  ana\nbob\r\n\tcarla  ", ["ana", "bob", "carla"]],
    ["bob, ana, bob", ["bob", "ana"]],
    [",,  ,", []],
  ])("parseIDs(%j)", (text, want) => {
    expect(parseIDs(text)).toEqual(want)
  })
})

describe("loadAll", () => {
  const pages: Record<string, { items: number[]; next_after?: string }> = {
    start: { items: [1, 2], next_after: "2" },
    "2": { items: [3, 4], next_after: "4" },
    "4": { items: [5] },
  }
  const fetchPage = async (after?: string) => pages[after ?? "start"]

  it("follows the pages to the last", async () => {
    await expect(loadAll(fetchPage, 10)).resolves.toEqual({ items: [1, 2, 3, 4, 5], complete: true })
  })

  it("stops at the limit and says there is more", async () => {
    await expect(loadAll(fetchPage, 2)).resolves.toEqual({ items: [1, 2, 3, 4], complete: false })
  })
})

describe("formats", () => {
  it.each([
    [0, "0"],
    [999, "999"],
    [1234, "1.2K"],
    [16_000_000, "16M"],
  ])("formatCount(%d)", (value, want) => {
    expect(formatCount(value)).toBe(want)
  })

  it.each([
    [0, "0/s"],
    [0.25, "0.25/s"],
    [12.34, "12.3/s"],
    [1850, "1.9K/s"],
  ])("formatRate(%d)", (value, want) => {
    expect(formatRate(value)).toBe(want)
  })

  it.each([
    [0.0251, "25 ms"],
    [0.5, "500 ms"],
    [1.256, "1.26 s"],
  ])("formatSeconds(%d)", (value, want) => {
    expect(formatSeconds(value)).toBe(want)
  })

  it.each([
    [512, "512 B"],
    [2048, "2.0 KB"],
    [52_428_800, "50 MB"],
  ])("formatBytes(%d)", (value, want) => {
    expect(formatBytes(value)).toBe(want)
  })
})
