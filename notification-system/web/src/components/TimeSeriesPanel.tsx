import { CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts"

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { ChartContainer, ChartTooltip } from "@/components/ui/chart"
import type { Series } from "@/lib/api"
import { formatClock } from "@/lib/format"

/** One line a panel can draw: the value of the label that selects its series, its name and its color. */
export type LineSpec = { key: string; label: string; color: string }

/**
 * A panel with one chart of values over time.
 *
 * `lines` fixes which series are drawn, in which order and color, whatever data arrives:
 * a line that has no data yet keeps its place in the legend, and a series keeps its color
 * from one refresh to the next. `by` names the label whose value is a line's key.
 */
export function TimeSeriesPanel(props: {
  title: string
  description?: string
  series: Series[]
  by: string
  lines: LineSpec[]
  format: (value: number) => string
}) {
  const { lines, format } = props
  const rows = toRows(props.series, props.by, lines)
  const span = rows.length > 1 ? rows[rows.length - 1].t - rows[0].t : 0

  return (
    <Card>
      <CardHeader>
        <CardTitle>{props.title}</CardTitle>
        {props.description && <CardDescription>{props.description}</CardDescription>}
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {lines.length > 1 && (
          <ul className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
            {lines.map((line) => (
              <li key={line.key} className="flex items-center gap-1.5">
                <span className="h-0.5 w-3 rounded-full" style={{ background: line.color }} aria-hidden />
                {line.label}
              </li>
            ))}
          </ul>
        )}
        {rows.length === 0 ? (
          <p className="flex h-48 items-center justify-center text-sm text-muted-foreground">Nothing recorded in this range.</p>
        ) : (
          <ChartContainer config={{}} className="aspect-auto h-48 w-full">
            <LineChart data={rows} margin={{ top: 4, right: 8, bottom: 0, left: 0 }} accessibilityLayer>
              <CartesianGrid vertical={false} />
              <XAxis
                dataKey="t"
                type="number"
                domain={["dataMin", "dataMax"]}
                tickFormatter={(t: number) => formatClock(t, span < 600)}
                tickLine={false}
                axisLine={false}
                minTickGap={48}
              />
              <YAxis width={56} tickFormatter={format} tickLine={false} axisLine={false} allowDecimals domain={[0, "auto"]} />
              <ChartTooltip
                isAnimationActive={false}
                content={({ active, payload, label }) =>
                  active && payload?.length ? (
                    <div className="grid gap-1.5 rounded-lg border bg-background px-2.5 py-1.5 text-xs shadow-md">
                      <p className="font-medium">{formatClock(Number(label), true)}</p>
                      {lines.map((line) => {
                        const value = payload.find((item) => item.dataKey === line.key)?.value
                        return typeof value === "number" ? (
                          <p key={line.key} className="flex items-center gap-2">
                            <span className="size-2 rounded-full" style={{ background: line.color }} aria-hidden />
                            <span className="text-muted-foreground">{line.label}</span>
                            <span className="ml-auto pl-4 font-medium">{format(value)}</span>
                          </p>
                        ) : null
                      })}
                    </div>
                  ) : null
                }
              />
              {lines.map((line) => (
                <Line
                  key={line.key}
                  dataKey={line.key}
                  name={line.label}
                  stroke={line.color}
                  strokeWidth={2}
                  dot={false}
                  activeDot={{ r: 4 }}
                  type="linear"
                  // The data is replaced every few seconds; animating each time would never settle.
                  isAnimationActive={false}
                />
              ))}
            </LineChart>
          </ChartContainer>
        )}
      </CardContent>
    </Card>
  )
}

type Row = { t: number } & Record<string, number>

/** Turns series into one row per moment, with a column per line. Series that are not a line are dropped. */
function toRows(series: Series[], by: string, lines: LineSpec[]): Row[] {
  const rows = new Map<number, Row>()
  for (const one of series) {
    const key = one.labels[by] ?? lines[0]?.key
    if (!lines.some((line) => line.key === key)) {
      continue
    }
    for (const [t, value] of one.points) {
      const row = rows.get(t) ?? ({ t } as Row)
      row[key] = value
      rows.set(t, row)
    }
  }
  return [...rows.values()].sort((a, b) => a.t - b.t)
}
