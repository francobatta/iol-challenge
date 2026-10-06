import { ActivityIcon, BookOpenIcon, ListIcon, LogOutIcon, SendIcon, UsersIcon } from "lucide-react"
import { NavLink, Outlet } from "react-router"
import { cn } from "cn"

import { Button } from "@/components/ui/button"
import { useAuth } from "@/lib/auth"

const sections = [
  { to: "/", label: "Dashboard", icon: ActivityIcon },
  { to: "/users", label: "Users", icon: UsersIcon },
  { to: "/lists", label: "Lists", icon: ListIcon },
  { to: "/notifications", label: "Notifications", icon: SendIcon },
  { to: "/docs", label: "API docs", icon: BookOpenIcon },
]

/** The frame around every page once an app is signed in: where to go, and who is signed in. */
export function AppShell() {
  const { appID, signOut } = useAuth()
  return (
    <div className="flex min-h-svh flex-col md:flex-row">
      <aside className="flex shrink-0 flex-col gap-4 border-b bg-sidebar p-3 md:sticky md:top-0 md:h-svh md:w-56 md:border-r md:border-b-0 md:p-4">
        <p className="hidden px-2 text-sm font-semibold md:block">Notification console</p>
        <nav aria-label="Sections" className="flex gap-1 overflow-x-auto md:flex-col">
          {sections.map(({ to, label, icon: Icon }) => (
            <NavLink
              key={to}
              to={to}
              end={to === "/"}
              className={({ isActive }) =>
                cn(
                  "flex items-center gap-2 rounded-md px-2 py-1.5 text-sm whitespace-nowrap text-muted-foreground outline-none hover:bg-sidebar-accent hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50",
                  isActive && "bg-sidebar-accent font-medium text-foreground",
                )
              }
            >
              <Icon className="size-4" aria-hidden />
              {label}
            </NavLink>
          ))}
        </nav>
        <div className="flex items-center justify-between gap-2 md:mt-auto md:flex-col md:items-stretch">
          <div className="min-w-0 px-2 text-xs">
            <p className="text-muted-foreground">Signed in as app</p>
            <p className="truncate" title={appID ?? undefined}>
              {appID ?? "unknown"}
            </p>
          </div>
          <Button variant="outline" size="sm" onClick={signOut}>
            <LogOutIcon aria-hidden />
            Sign out
          </Button>
        </div>
      </aside>
      <main className="min-w-0 flex-1 px-4 py-6 md:px-8 md:py-8">
        <div className="mx-auto flex max-w-6xl flex-col gap-6">
          <Outlet />
        </div>
      </main>
    </div>
  )
}
