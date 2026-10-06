import { lazy, Suspense } from "react"
import { ChevronLeftIcon } from "lucide-react"
import { Link } from "react-router"

import { useAuth } from "@/lib/auth"

// Redoc is large and only this page uses it, so it is loaded when the page is opened.
const Reference = lazy(() => import("@/components/Reference"))

/** Height of the bar above the reference, which Redoc must leave room for when it scrolls to a section. */
const barHeight = 44

/**
 * How to use the API: a guide and the reference of every route, drawn by Redoc from
 * /openapi.yaml. It needs no sign-in, since it is what someone reads before having an app.
 */
export default function Docs() {
  const { token } = useAuth()
  return (
    // Redoc draws itself light whatever the system setting, so the page around it does too.
    <div className="min-h-svh bg-white text-neutral-950">
      <header
        className="sticky top-0 z-10 flex items-center justify-between border-b border-neutral-200 bg-white px-4 text-sm"
        style={{ height: barHeight }}
      >
        <span className="font-semibold">Notification system docs</span>
        <Link to="/" className="flex items-center gap-1 text-neutral-600 hover:text-neutral-950">
          <ChevronLeftIcon className="size-4" aria-hidden />
          {token ? "Back to the console" : "Sign in to the console"}
        </Link>
      </header>
      <Suspense fallback={<p className="p-8 text-sm text-neutral-600">Loading the docs…</p>}>
        <Reference scrollOffset={barHeight} />
      </Suspense>
    </div>
  )
}
