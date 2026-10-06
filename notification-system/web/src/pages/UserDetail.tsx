import { useState, type FormEvent } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Trash2Icon } from "lucide-react"
import { useParams } from "react-router"
import { toast } from "sonner"

import { ErrorNote, Field, LoadMore, PageHeader, TableStatus } from "@/components/common"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { api, channelProviders, type Channel } from "@/lib/api"
import { usePages } from "@/lib/pages"

const channels: { value: Channel; label: string; address: string; placeholder: string }[] = [
  { value: "sms", label: "SMS", address: "Phone number", placeholder: "+5491100000000" },
  { value: "email", label: "Email", address: "Email address", placeholder: "ana@example.com" },
  { value: "push", label: "Push", address: "Device token", placeholder: "device-token" },
]

export default function UserDetail() {
  const userID = useParams().userID!
  const queries = useQueryClient()
  const key = ["users", userID, "endpoints"]
  const endpoints = usePages(key, (after) => api.endpoints(userID, after))

  const remove = useMutation({
    mutationFn: api.deleteEndpoint,
    onSuccess: () => {
      toast.success("Deleted endpoint")
      return queries.invalidateQueries({ queryKey: key })
    },
    onError: (error) => toast.error(error.message),
  })

  return (
    <>
      <PageHeader
        title={userID}
        back={{ to: "/users", label: "Users" }}
        description="A notification to this user goes to every endpoint below, each through its provider."
      />
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Address</TableHead>
            <TableHead>Channel</TableHead>
            <TableHead>Provider</TableHead>
            <TableHead className="w-0" />
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableStatus columns={4} loading={endpoints.isPending} error={endpoints.error} empty={endpoints.items.length === 0}>
            No endpoints yet, so notifications to this user reach nobody. Add one below.
          </TableStatus>
          {endpoints.items.map((endpoint) => (
            <TableRow key={endpoint.endpoint_id}>
              <TableCell className="font-medium break-all">{endpoint.address}</TableCell>
              <TableCell>
                <Badge variant="secondary">{channels.find((channel) => channel.value === endpoint.channel)?.label}</Badge>
              </TableCell>
              <TableCell>{endpoint.provider}</TableCell>
              <TableCell>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={`Delete endpoint ${endpoint.address}`}
                  disabled={remove.isPending}
                  onClick={() => remove.mutate(endpoint.endpoint_id)}
                >
                  <Trash2Icon />
                </Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <LoadMore hasMore={endpoints.hasNextPage} loading={endpoints.isFetchingNextPage} onLoad={() => endpoints.fetchNextPage()} />
      <AddEndpoint userID={userID} onAdded={() => queries.invalidateQueries({ queryKey: key })} />
    </>
  )
}

function AddEndpoint({ userID, onAdded }: { userID: string; onAdded: () => void }) {
  const [channel, setChannel] = useState<Channel>("sms")
  const [provider, setProvider] = useState(channelProviders.sms[0])
  const [address, setAddress] = useState("")
  const selected = channels.find((option) => option.value === channel)!

  const add = useMutation({
    mutationFn: () => api.createEndpoint(userID, { address: address.trim(), channel, provider }),
    onSuccess: () => {
      toast.success("Added endpoint")
      setAddress("")
      onAdded()
    },
  })

  function chooseChannel(next: Channel) {
    setChannel(next)
    setProvider(channelProviders[next][0]) // the provider of another channel does not deliver on this one
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    add.mutate()
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Add an endpoint</CardTitle>
        <CardDescription>The channel decides which providers can deliver to it.</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={submit} className="flex flex-col gap-4">
          <div className="grid gap-4 sm:grid-cols-[10rem_10rem_1fr]">
            <Field label="Channel" htmlFor="channel">
              <Select items={channels} value={channel} onValueChange={(next) => next && chooseChannel(next as Channel)}>
                <SelectTrigger id="channel" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {channels.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field label="Provider" htmlFor="provider">
              <Select value={provider} onValueChange={(next) => next && setProvider(next as string)}>
                <SelectTrigger id="provider" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {channelProviders[channel].map((name) => (
                    <SelectItem key={name} value={name}>
                      {name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field label={selected.address} htmlFor="address">
              <Input
                id="address"
                required
                value={address}
                onChange={(event) => setAddress(event.target.value)}
                placeholder={selected.placeholder}
              />
            </Field>
          </div>
          <ErrorNote error={add.error} />
          <Button type="submit" className="self-start" disabled={add.isPending || address.trim() === ""}>
            Add endpoint
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
