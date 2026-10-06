import { useState, type FormEvent } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { SendIcon, XIcon } from "lucide-react"
import { Link, useParams } from "react-router"
import { toast } from "sonner"
import { cn } from "cn"

import { ErrorNote, LoadMore, PageHeader, TableStatus } from "@/components/common"
import { UserIDsField } from "@/components/UserIDsField"
import { Button, buttonVariants } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api, maxUserIDs } from "@/lib/api"
import { formatDateTime } from "@/lib/format"
import { parseIDs } from "@/lib/ids"
import { usePages } from "@/lib/pages"

export default function ListDetail() {
  const listID = useParams().listID!
  const queries = useQueryClient()
  const key = ["lists", listID, "members"]
  const list = useQuery({ queryKey: ["lists", listID], queryFn: () => api.list(listID) })
  const members = usePages(key, (after) => api.members(listID, after))

  const remove = useMutation({
    mutationFn: (userID: string) => api.removeMember(listID, userID),
    onSuccess: (_, userID) => {
      toast.success(`Removed ${userID} from the list`)
      return queries.invalidateQueries({ queryKey: key })
    },
    onError: (error) => toast.error(error.message),
  })

  return (
    <>
      <PageHeader
        title={list.data?.name ?? "List"}
        back={{ to: "/lists", label: "Lists" }}
        description={list.data?.description}
        action={
          <Link to={`/notifications?list=${listID}`} className={cn(buttonVariants({ variant: "outline" }))}>
            <SendIcon aria-hidden />
            Notify this list
          </Link>
        }
      />
      <ErrorNote error={list.error} />
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>User ID</TableHead>
            <TableHead>Added</TableHead>
            <TableHead className="w-0" />
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableStatus columns={3} loading={members.isPending} error={members.error} empty={members.items.length === 0}>
            Nobody is on this list yet. Add users below.
          </TableStatus>
          {members.items.map((member) => (
            <TableRow key={member.user_id}>
              <TableCell>
                <Link to={`/users/${encodeURIComponent(member.user_id)}`} className="font-medium underline-offset-4 hover:underline">
                  {member.user_id}
                </Link>
              </TableCell>
              <TableCell className="text-muted-foreground">{formatDateTime(member.added_at)}</TableCell>
              <TableCell>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={`Remove ${member.user_id} from the list`}
                  disabled={remove.isPending}
                  onClick={() => remove.mutate(member.user_id)}
                >
                  <XIcon />
                </Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <LoadMore hasMore={members.hasNextPage} loading={members.isFetchingNextPage} onLoad={() => members.fetchNextPage()} />
      <AddMembers listID={listID} onAdded={() => queries.invalidateQueries({ queryKey: key })} />
    </>
  )
}

function AddMembers({ listID, onAdded }: { listID: string; onAdded: () => void }) {
  const [text, setText] = useState("")
  const userIDs = parseIDs(text)
  const add = useMutation({
    mutationFn: () => api.addMembers(listID, userIDs),
    onSuccess: () => {
      toast.success(`Added ${userIDs.length} ${userIDs.length === 1 ? "user" : "users"} to the list`)
      setText("")
      onAdded()
    },
  })

  function submit(event: FormEvent) {
    event.preventDefault()
    add.mutate()
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Add users</CardTitle>
        <CardDescription>All of them are added, or none if one is not a registered user.</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={submit} className="flex flex-col gap-4">
          <UserIDsField id="member-ids" label="User IDs" value={text} onChange={setText} />
          <ErrorNote error={add.error} />
          <Button type="submit" className="self-start" disabled={add.isPending || userIDs.length === 0 || userIDs.length > maxUserIDs}>
            Add users
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
