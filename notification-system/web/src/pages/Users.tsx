import { useState, type FormEvent } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { PlusIcon, Trash2Icon, UploadIcon } from "lucide-react"
import { Link, useNavigate } from "react-router"
import { toast } from "sonner"

import { ConfirmDialog, ErrorNote, Field, LoadMore, PageHeader, TableStatus } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api } from "@/lib/api"
import { formatDateTime } from "@/lib/format"
import { usePages } from "@/lib/pages"

export default function Users() {
  const queries = useQueryClient()
  const users = usePages(["users"], api.users)
  const [registering, setRegistering] = useState(false)
  const [importing, setImporting] = useState(false)
  const [deleting, setDeleting] = useState<string | null>(null)

  const remove = useMutation({
    mutationFn: api.deleteUser,
    onSuccess: (_, userID) => {
      toast.success(`Deleted user ${userID}`)
      setDeleting(null)
      return queries.invalidateQueries({ queryKey: ["users"] })
    },
  })

  return (
    <>
      <PageHeader
        title="Users"
        description="The people your app can notify. Each one is reached through the endpoints registered for them."
        action={
          <div className="flex gap-2">
            <Button variant="outline" onClick={() => setImporting(true)}>
              <UploadIcon aria-hidden />
              Import
            </Button>
            <Button onClick={() => setRegistering(true)}>
              <PlusIcon aria-hidden />
              Register user
            </Button>
          </div>
        }
      />
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>User ID</TableHead>
            <TableHead>Registered</TableHead>
            <TableHead className="w-0" />
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableStatus columns={3} loading={users.isPending} error={users.error} empty={users.items.length === 0}>
            No users yet. Register the first one to start building an audience.
          </TableStatus>
          {users.items.map((user) => (
            <TableRow key={user.user_id}>
              <TableCell>
                <Link to={`/users/${encodeURIComponent(user.user_id)}`} className="font-medium underline-offset-4 hover:underline">
                  {user.user_id}
                </Link>
              </TableCell>
              <TableCell className="text-muted-foreground">{formatDateTime(user.created_at)}</TableCell>
              <TableCell>
                <Button variant="ghost" size="icon-sm" aria-label={`Delete user ${user.user_id}`} onClick={() => setDeleting(user.user_id)}>
                  <Trash2Icon />
                </Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <LoadMore hasMore={users.hasNextPage} loading={users.isFetchingNextPage} onLoad={() => users.fetchNextPage()} />

      <RegisterUserDialog open={registering} onClose={() => setRegistering(false)} />
      <ImportUsersDialog open={importing} onClose={() => setImporting(false)} />
      <ConfirmDialog
        open={deleting !== null}
        title={`Delete user ${deleting ?? ""}?`}
        description="Their endpoints and list memberships are deleted with them."
        action="Delete user"
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

function RegisterUserDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const queries = useQueryClient()
  const navigate = useNavigate()
  const [userID, setUserID] = useState("")
  const register = useMutation({
    mutationFn: api.registerUser,
    onSuccess: async (user) => {
      await queries.invalidateQueries({ queryKey: ["users"] })
      toast.success(`Registered user ${user.user_id}`)
      close()
      // A user is no use without an endpoint, and that is where they are added.
      navigate(`/users/${encodeURIComponent(user.user_id)}`)
    },
  })

  function close() {
    setUserID("")
    register.reset()
    onClose()
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    register.mutate(userID.trim())
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !next && close()}>
      <DialogContent>
        <form onSubmit={submit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>Register user</DialogTitle>
            <DialogDescription>Use the ID your app already knows this person by.</DialogDescription>
          </DialogHeader>
          <Field label="User ID" htmlFor="user-id">
            <Input id="user-id" required autoFocus value={userID} onChange={(event) => setUserID(event.target.value)} placeholder="ana" />
          </Field>
          <ErrorNote error={register.error} />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={close}>
              Cancel
            </Button>
            <Button type="submit" disabled={register.isPending || userID.trim() === ""}>
              Register user
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

const noList = "none"

function ImportUsersDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const queries = useQueryClient()
  const [file, setFile] = useState<File | null>(null)
  const [listID, setListID] = useState(noList)

  // The select offers the first lists only, as the one that sends a notification does.
  const lists = useQuery({ queryKey: ["lists", "options"], queryFn: () => api.lists(undefined, 200), enabled: open })
  const listOptions = [
    { value: noList, label: "No list" },
    ...(lists.data?.items ?? []).map((list) => ({ value: list.list_id, label: list.name })),
  ]

  const run = useMutation({
    mutationFn: (csv: File) => api.importUsers(csv, listID === noList ? undefined : listID),
    onSuccess: (result) => {
      const count = (n: number) => n.toLocaleString("en-US")
      toast.success(`Imported ${count(result.rows)} rows: ${count(result.users)} new users, ${count(result.endpoints)} new endpoints`)
      close()
    },
    // An import that fails keeps the rows stored before the one that stopped it.
    onSettled: () => queries.invalidateQueries(),
  })

  function close() {
    setFile(null)
    setListID(noList)
    run.reset()
    onClose()
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    if (file) {
      run.mutate(file)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !next && close()}>
      <DialogContent>
        <form onSubmit={submit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>Import users</DialogTitle>
            <DialogDescription>
              A CSV with one endpoint per row and no header: <code>user_id,channel,provider,address</code>. Users and endpoints that
              already exist are left as they are.
            </DialogDescription>
          </DialogHeader>
          <Field
            label="File"
            htmlFor="import-file"
            hint={
              <>
                Nothing to import?{" "}
                <a href="/sample-users.csv" download className="underline underline-offset-4 hover:text-foreground">
                  Download a sample of 5,000 users
                </a>
                .
              </>
            }
          >
            <Input id="import-file" type="file" accept=".csv,text/csv" required onChange={(event) => setFile(event.target.files?.[0] ?? null)} />
          </Field>
          <Field label="Add to list" htmlFor="import-list" hint="Optional. Every user in the file joins it.">
            <Select items={listOptions} value={listID} onValueChange={(next) => next && setListID(next as string)}>
              <SelectTrigger id="import-list" className="w-full">
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
          <ErrorNote error={run.error} />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={close}>
              Cancel
            </Button>
            <Button type="submit" disabled={run.isPending || !file}>
              {run.isPending ? "Importing…" : "Import"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
