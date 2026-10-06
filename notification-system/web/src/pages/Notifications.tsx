import { useState, type FormEvent } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useSearchParams } from "react-router"
import { toast } from "sonner"

import { ErrorNote, Field, PageHeader, TableStatus } from "@/components/common"
import { UserIDsField } from "@/components/UserIDsField"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import { api, maxUserIDs, type Job, type JobStatus, type Priority } from "@/lib/api"
import { formatDateTime, formatExact } from "@/lib/format"
import { parseIDs } from "@/lib/ids"
import { loadAll } from "@/lib/pages"

export default function Notifications() {
  return (
    <>
      <PageHeader
        title="Notifications"
        description="A notification becomes a job, which is dispatched into one delivery per endpoint. The dashboard shows what became of the deliveries."
      />
      <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,24rem)_minmax(0,1fr)]">
        <SendForm />
        <Jobs />
      </div>
    </>
  )
}

const noList = "none"
const priorities: { value: Priority; label: string }[] = [
  { value: "normal", label: "Normal" },
  { value: "high", label: "High" },
]

function SendForm() {
  const queries = useQueryClient()
  const [params] = useSearchParams()
  const [listID, setListID] = useState(params.get("list") ?? noList)
  const [idsText, setIDsText] = useState("")
  const [priority, setPriority] = useState<Priority>("normal")
  const [title, setTitle] = useState("")
  const [body, setBody] = useState("")
  // One key per notification the form is filled in for: sending it twice, by a double
  // click or a retry after a lost answer, creates one job.
  const [idempotencyKey, setIdempotencyKey] = useState(() => crypto.randomUUID())

  // The select offers the first lists only; an app with more sends to one from its page.
  const lists = useQuery({ queryKey: ["lists", "options"], queryFn: () => api.lists(undefined, 200) })
  const listOptions = [
    { value: noList, label: "No list" },
    ...(lists.data?.items ?? []).map((list) => ({ value: list.list_id, label: list.name })),
  ]

  const userIDs = parseIDs(idsText)
  const hasAudience = listID !== noList || userIDs.length > 0

  const send = useMutation({
    mutationFn: () =>
      api.sendNotification(
        {
          list_id: listID === noList ? undefined : listID,
          user_ids: userIDs.length > 0 ? userIDs : undefined,
          priority,
          content: { title: title.trim(), body: body.trim() },
        },
        idempotencyKey,
      ),
    onSuccess: () => {
      toast.success("Notification accepted")
      setIdempotencyKey(crypto.randomUUID())
      setTitle("")
      setBody("")
      setIDsText("")
      return queries.invalidateQueries({ queryKey: ["jobs"] })
    },
  })

  function submit(event: FormEvent) {
    event.preventDefault()
    send.mutate()
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Send a notification</CardTitle>
      </CardHeader>
      <CardContent>
        <form onSubmit={submit} className="flex flex-col gap-4">
          <Field label="List" htmlFor="list" hint="Everyone on the list, plus the users named below.">
            <Select items={listOptions} value={listID} onValueChange={(next) => next && setListID(next as string)}>
              <SelectTrigger id="list" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {listOptions.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <UserIDsField id="user-ids" label="Individual users" value={idsText} onChange={setIDsText} />
          <Field label="Priority" htmlFor="priority" hint="High goes ahead of your app's own normal deliveries.">
            <Select items={priorities} value={priority} onValueChange={(next) => next && setPriority(next as Priority)}>
              <SelectTrigger id="priority" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {priorities.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Field label="Title" htmlFor="title" hint="Optional.">
            <Input id="title" value={title} onChange={(event) => setTitle(event.target.value)} />
          </Field>
          <Field label="Message" htmlFor="body">
            <Textarea id="body" rows={3} required value={body} onChange={(event) => setBody(event.target.value)} />
          </Field>
          <ErrorNote error={send.error} />
          {!hasAudience && <p className="text-sm text-muted-foreground">Choose a list or name at least one user.</p>}
          <Button type="submit" disabled={send.isPending || !hasAudience || userIDs.length > maxUserIDs || body.trim() === ""}>
            {send.isPending ? "Sending…" : "Send notification"}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}

/** The most jobs the table loads: ten pages of the API's largest. */
const maxJobPages = 10

const statusLabels: Record<JobStatus, string> = { pending: "Pending", dispatching: "Dispatching", dispatched: "Dispatched" }

function Jobs() {
  const jobs = useQuery({
    queryKey: ["jobs"],
    // The API lists jobs oldest first and the latest are the ones of interest, so all of
    // them are loaded to show them in reverse.
    queryFn: () => loadAll((after) => api.jobs(after, 200), maxJobPages),
    // Keep asking while a job is still on its way to being dispatched.
    refetchInterval: (query) => (query.state.data?.items.some((job) => job.status !== "dispatched") ? 2000 : false),
  })
  const latestFirst = [...(jobs.data?.items ?? [])].reverse()

  return (
    <div className="flex min-w-0 flex-col gap-2">
      <h2 className="font-medium">Jobs</h2>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Accepted</TableHead>
            <TableHead>Status</TableHead>
            <TableHead>Priority</TableHead>
            <TableHead className="text-right">Deliveries queued</TableHead>
            <TableHead>Job ID</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableStatus columns={5} loading={jobs.isPending} error={jobs.error} empty={latestFirst.length === 0}>
            No notifications sent yet.
          </TableStatus>
          {latestFirst.map((job) => (
            <JobRow key={job.job_id} job={job} />
          ))}
        </TableBody>
      </Table>
      {jobs.data && !jobs.data.complete && (
        <p className="text-sm text-muted-foreground">
          Showing the first {formatExact(jobs.data.items.length)} jobs. Later ones are not listed here.
        </p>
      )}
    </div>
  )
}

function JobRow({ job }: { job: Job }) {
  return (
    <TableRow>
      <TableCell className="whitespace-nowrap">{formatDateTime(job.created_at)}</TableCell>
      <TableCell>
        <Badge variant={job.status === "dispatched" ? "secondary" : "outline"}>{statusLabels[job.status]}</Badge>
      </TableCell>
      <TableCell>{job.priority === "high" ? "High" : "Normal"}</TableCell>
      <TableCell className="text-right">{formatExact(job.queued)}</TableCell>
      <TableCell className="text-muted-foreground" title={job.job_id}>
        <span className="block max-w-28 truncate">{job.job_id}</span>
      </TableCell>
    </TableRow>
  )
}
