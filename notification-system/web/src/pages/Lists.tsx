import { useState, type FormEvent } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { PlusIcon, Trash2Icon } from "lucide-react"
import { Link, useNavigate } from "react-router"
import { toast } from "sonner"

import { ConfirmDialog, ErrorNote, Field, LoadMore, PageHeader, TableStatus } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api, type List } from "@/lib/api"
import { formatDateTime } from "@/lib/format"
import { usePages } from "@/lib/pages"

export default function Lists() {
  const queries = useQueryClient()
  const lists = usePages(["lists"], (after) => api.lists(after))
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState<List | null>(null)

  const remove = useMutation({
    mutationFn: (list: List) => api.deleteList(list.list_id),
    onSuccess: (_, list) => {
      toast.success(`Deleted list ${list.name}`)
      setDeleting(null)
      return queries.invalidateQueries({ queryKey: ["lists"] })
    },
  })

  return (
    <>
      <PageHeader
        title="Lists"
        description="Named groups of users. A notification sent to a list goes to everyone on it at that moment."
        action={
          <Button onClick={() => setCreating(true)}>
            <PlusIcon aria-hidden />
            Create list
          </Button>
        }
      />
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Description</TableHead>
            <TableHead>Created</TableHead>
            <TableHead className="w-0" />
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableStatus columns={4} loading={lists.isPending} error={lists.error} empty={lists.items.length === 0}>
            No lists yet. Create one to notify a group of users at once.
          </TableStatus>
          {lists.items.map((list) => (
            <TableRow key={list.list_id}>
              <TableCell>
                <Link to={`/lists/${list.list_id}`} className="font-medium underline-offset-4 hover:underline">
                  {list.name}
                </Link>
              </TableCell>
              <TableCell className="max-w-xs truncate text-muted-foreground">{list.description}</TableCell>
              <TableCell className="text-muted-foreground">{formatDateTime(list.created_at)}</TableCell>
              <TableCell>
                <Button variant="ghost" size="icon-sm" aria-label={`Delete list ${list.name}`} onClick={() => setDeleting(list)}>
                  <Trash2Icon />
                </Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <LoadMore hasMore={lists.hasNextPage} loading={lists.isFetchingNextPage} onLoad={() => lists.fetchNextPage()} />

      <CreateListDialog open={creating} onClose={() => setCreating(false)} />
      <ConfirmDialog
        open={deleting !== null}
        title={`Delete list ${deleting?.name ?? ""}?`}
        description="The users on it stay registered; only the list goes."
        action="Delete list"
        pending={remove.isPending}
        error={remove.error}
        onConfirm={() => deleting && remove.mutate(deleting)}
        onClose={() => {
          setDeleting(null)
          remove.reset()
        }}
      />
    </>
  )
}

function CreateListDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const queries = useQueryClient()
  const navigate = useNavigate()
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const create = useMutation({
    mutationFn: () => api.createList({ name: name.trim(), description: description.trim() }),
    onSuccess: async (list) => {
      await queries.invalidateQueries({ queryKey: ["lists"] })
      toast.success(`Created list ${list.name}`)
      close()
      navigate(`/lists/${list.list_id}`) // where its members are added
    },
  })

  function close() {
    setName("")
    setDescription("")
    create.reset()
    onClose()
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    create.mutate()
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !next && close()}>
      <DialogContent>
        <form onSubmit={submit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>Create list</DialogTitle>
            <DialogDescription>Add its members on the next page.</DialogDescription>
          </DialogHeader>
          <Field label="Name" htmlFor="list-name">
            <Input id="list-name" required autoFocus value={name} onChange={(event) => setName(event.target.value)} placeholder="beta-testers" />
          </Field>
          <Field label="Description" htmlFor="list-description" hint="Optional.">
            <Input id="list-description" value={description} onChange={(event) => setDescription(event.target.value)} />
          </Field>
          <ErrorNote error={create.error} />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={close}>
              Cancel
            </Button>
            <Button type="submit" disabled={create.isPending || name.trim() === ""}>
              Create list
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
