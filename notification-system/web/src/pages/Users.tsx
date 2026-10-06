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
import { api } from "@/lib/api"
import { formatDateTime } from "@/lib/format"
import { usePages } from "@/lib/pages"

export default function Users() {
  const queries = useQueryClient()
  const users = usePages(["users"], api.users)
  const [registering, setRegistering] = useState(false)
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
          <Button onClick={() => setRegistering(true)}>
            <PlusIcon aria-hidden />
            Register user
          </Button>
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
