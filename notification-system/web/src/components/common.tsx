// The small pieces every page is put together from.

import type { ReactNode } from "react"
import { ChevronLeftIcon } from "lucide-react"
import { Link } from "react-router"

import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Label } from "@/components/ui/label"
import { TableCell, TableRow } from "@/components/ui/table"

/** The title of a page, with the way back if it is a page about one thing, and its main action. */
export function PageHeader(props: { title: string; description?: ReactNode; back?: { to: string; label: string }; action?: ReactNode }) {
  return (
    <header className="flex flex-col gap-2">
      {props.back && (
        <Link to={props.back.to} className="flex w-fit items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
          <ChevronLeftIcon className="size-4" aria-hidden />
          {props.back.label}
        </Link>
      )}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="text-xl font-semibold tracking-tight break-words">{props.title}</h1>
          {props.description && <p className="mt-1 max-w-prose text-sm text-muted-foreground">{props.description}</p>}
        </div>
        {props.action}
      </div>
    </header>
  )
}

/** What went wrong with a request, in the words of the API. */
export function ErrorNote({ error }: { error: unknown }) {
  if (!error) {
    return null
  }
  return (
    <p role="alert" className="text-sm text-destructive">
      {messageOf(error)}
    </p>
  )
}

export function messageOf(error: unknown): string {
  return error instanceof Error ? error.message : "Something went wrong."
}

/** A labelled control of a form. */
export function Field(props: { label: string; htmlFor: string; hint?: ReactNode; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={props.htmlFor}>{props.label}</Label>
      {props.children}
      {props.hint && <p className="text-xs text-muted-foreground">{props.hint}</p>}
    </div>
  )
}

/** The row a table shows in place of its items while they load, fail to, or do not exist. */
export function TableStatus(props: { columns: number; loading: boolean; error: unknown; empty: boolean; children: ReactNode }) {
  let content: ReactNode = null
  if (props.loading) {
    content = "Loading…"
  } else if (props.error) {
    content = <ErrorNote error={props.error} />
  } else if (props.empty) {
    content = props.children
  }
  if (!content) {
    return null
  }
  return (
    <TableRow className="hover:bg-transparent">
      <TableCell colSpan={props.columns} className="py-8 text-center text-muted-foreground">
        {content}
      </TableCell>
    </TableRow>
  )
}

/** The button under a table that loads its next page. Renders nothing on the last one. */
export function LoadMore(props: { hasMore: boolean; loading: boolean; onLoad: () => void }) {
  if (!props.hasMore) {
    return null
  }
  return (
    <Button variant="outline" className="self-center" disabled={props.loading} onClick={props.onLoad}>
      {props.loading ? "Loading…" : "Load more"}
    </Button>
  )
}

/** Asks before doing something that cannot be undone. `action` names it, and is what the button says. */
export function ConfirmDialog(props: {
  open: boolean
  title: string
  description: ReactNode
  action: string
  pending: boolean
  error?: unknown
  onConfirm: () => void
  onClose: () => void
}) {
  return (
    <Dialog open={props.open} onOpenChange={(open) => !open && props.onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{props.title}</DialogTitle>
          <DialogDescription>{props.description}</DialogDescription>
        </DialogHeader>
        <ErrorNote error={props.error} />
        <DialogFooter>
          <Button variant="outline" onClick={props.onClose}>
            Cancel
          </Button>
          <Button variant="destructive" disabled={props.pending} onClick={props.onConfirm}>
            {props.action}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
