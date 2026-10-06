import { useState, type FormEvent } from "react"
import { useMutation } from "@tanstack/react-query"
import { CheckIcon, CopyIcon } from "lucide-react"
import { Link } from "react-router"

import { ErrorNote, Field } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { checkToken, createApp } from "@/lib/api"
import { useAuth } from "@/lib/auth"

export default function Login() {
  return (
    <main className="flex min-h-svh items-start justify-center px-4 py-16 sm:items-center sm:py-8">
      <div className="flex w-full max-w-sm flex-col gap-6">
        <header>
          <h1 className="text-2xl font-semibold tracking-tight">Notification console</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Sign in as your app to manage its users and lists, send notifications and follow their delivery.
          </p>
        </header>
        <Tabs defaultValue="token">
          <TabsList className="w-full">
            <TabsTrigger value="token">Use a token</TabsTrigger>
            <TabsTrigger value="create">Create an app</TabsTrigger>
          </TabsList>
          <TabsContent value="token" className="pt-4">
            <TokenForm />
          </TabsContent>
          <TabsContent value="create" className="pt-4">
            <CreateAppForm />
          </TabsContent>
        </Tabs>
        <p className="text-sm text-muted-foreground">
          New to the API?{" "}
          <Link to="/docs" className="text-foreground underline underline-offset-4">
            Read the docs
          </Link>{" "}
          to go from creating an app to sending your first notification.
        </p>
      </div>
    </main>
  )
}

function TokenForm() {
  const { signIn } = useAuth()
  const [token, setToken] = useState("")
  const check = useMutation({
    mutationFn: async (candidate: string) => {
      if (!(await checkToken(candidate))) {
        throw new Error("The API does not accept this token. Check that all of it was pasted.")
      }
      return candidate
    },
    onSuccess: signIn,
  })

  function submit(event: FormEvent) {
    event.preventDefault()
    check.mutate(token.trim())
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <Field label="App token" htmlFor="token" hint="The token the API returned when the app was created.">
        <Textarea
          id="token"
          rows={4}
          required
          value={token}
          onChange={(event) => setToken(event.target.value)}
          placeholder="eyJhbGciOiJIUzI1NiIs…"
          spellCheck={false}
          className="break-all"
        />
      </Field>
      <ErrorNote error={check.error} />
      <Button type="submit" disabled={check.isPending || token.trim() === ""}>
        {check.isPending ? "Checking…" : "Sign in"}
      </Button>
    </form>
  )
}

function CreateAppForm() {
  const { signIn } = useAuth()
  const [name, setName] = useState("")
  const [adminKey, setAdminKey] = useState("")
  const [copied, setCopied] = useState(false)
  const create = useMutation({ mutationFn: () => createApp(name.trim(), adminKey) })

  function submit(event: FormEvent) {
    event.preventDefault()
    create.mutate()
  }

  // The API never shows a token again, so signing in waits until this one has been seen.
  if (create.data) {
    const { token, name: created } = create.data
    return (
      <div className="flex flex-col gap-4">
        <div>
          <h2 className="font-medium">App “{created}” created</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Keep this token somewhere safe. It is how the app signs in, and it is not shown again.
          </p>
        </div>
        <output className="rounded-lg border bg-muted/50 p-3 text-xs break-all select-all">{token}</output>
        <div className="flex gap-2">
          <Button
            variant="outline"
            className="flex-1"
            onClick={() => navigator.clipboard.writeText(token).then(() => setCopied(true))}
          >
            {copied ? <CheckIcon aria-hidden /> : <CopyIcon aria-hidden />}
            {copied ? "Copied" : "Copy token"}
          </Button>
          <Button className="flex-1" onClick={() => signIn(token)}>
            Sign in
          </Button>
        </div>
      </div>
    )
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <Field label="App name" htmlFor="app-name">
        <Input id="app-name" required value={name} onChange={(event) => setName(event.target.value)} placeholder="demo" />
      </Field>
      <Field label="Admin key" htmlFor="admin-key" hint="Authorizes creating apps. It is sent once and not kept.">
        <Input
          id="admin-key"
          type="password"
          required
          autoComplete="off"
          value={adminKey}
          onChange={(event) => setAdminKey(event.target.value)}
        />
      </Field>
      <ErrorNote error={create.error} />
      <Button type="submit" disabled={create.isPending}>
        {create.isPending ? "Creating…" : "Create app"}
      </Button>
    </form>
  )
}
