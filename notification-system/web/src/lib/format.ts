// How numbers and times are written across the console.

const count = new Intl.NumberFormat("en", { notation: "compact", maximumFractionDigits: 1 })
const precise = new Intl.NumberFormat("en", { maximumFractionDigits: 0 })

/** A number of things, shortened past the thousands: 1.2K, 3.4M. */
export function formatCount(value: number): string {
  return count.format(Math.round(value))
}

/** A number of things in full: 12,345. */
export function formatExact(value: number): string {
  return precise.format(Math.round(value))
}

/** Things per second, with more digits the smaller it is. */
export function formatRate(value: number): string {
  if (value === 0) return "0/s"
  if (value < 1) return `${value.toFixed(2)}/s`
  if (value < 100) return `${value.toFixed(1)}/s`
  return `${count.format(value)}/s`
}

/** A duration given in seconds: 240 ms, 1.25 s. */
export function formatSeconds(value: number): string {
  if (value < 1) return `${Math.round(value * 1000)} ms`
  return `${value.toFixed(2)} s`
}

export function formatBytes(value: number): string {
  const units = ["B", "KB", "MB", "GB"]
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  return `${value.toFixed(value < 10 && unit > 0 ? 1 : 0)} ${units[unit]}`
}

/** A share of one core, as a percentage. */
export function formatCPU(cores: number): string {
  return `${(cores * 100).toFixed(1)}%`
}

const clock = new Intl.DateTimeFormat("en", { hour: "2-digit", minute: "2-digit", hour12: false })
const clockWithSeconds = new Intl.DateTimeFormat("en", { hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false })
const dateTime = new Intl.DateTimeFormat("en", { dateStyle: "medium", timeStyle: "short" })

/** The time of day of a moment in Unix seconds. Seconds are shown when they tell points apart. */
export function formatClock(unixSeconds: number, withSeconds = false): string {
  return (withSeconds ? clockWithSeconds : clock).format(unixSeconds * 1000)
}

/** A timestamp from the API, as a date and time. */
export function formatDateTime(iso: string): string {
  return dateTime.format(new Date(iso))
}
