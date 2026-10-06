import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react"
import { useQueryClient } from "@tanstack/react-query"

import { setSession } from "@/lib/api"

const storageKey = "notify.token"

/**
 * Returns the ID of the app a token was issued to: the subject of the JWT. The token is
 * not verified, which only the API can do; an ID read this way is for display.
 */
export function appIDOf(token: string): string | null {
  try {
    const payload = token.split(".")[1].replace(/-/g, "+").replace(/_/g, "/")
    const subject = JSON.parse(atob(payload)).sub
    return typeof subject === "string" && subject !== "" ? subject : null
  } catch {
    return null
  }
}

type Auth = {
  /** The token of the app that is signed in, or null. */
  token: string | null
  appID: string | null
  signIn: (token: string) => void
  signOut: () => void
}

const AuthContext = createContext<Auth | null>(null)

function storedToken() {
  try {
    return localStorage.getItem(storageKey)
  } catch {
    return null // storage is blocked; signing in lasts until the page is closed
  }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const queries = useQueryClient()
  const [token, setToken] = useState(storedToken)

  const signOut = useCallback(() => {
    try {
      localStorage.removeItem(storageKey)
    } catch {
      // nothing was stored
    }
    setToken(null)
    queries.clear() // what was loaded belongs to the app that just left
  }, [queries])

  const signIn = useCallback((next: string) => {
    try {
      localStorage.setItem(storageKey, next)
    } catch {
      // not remembered across reloads
    }
    setToken(next)
  }, [])

  // Set while rendering, not in an effect: the pages below make calls in their first one.
  setSession(token ? { token, onUnauthorized: signOut } : null)

  const value = useMemo(
    () => ({ token, appID: token ? appIDOf(token) : null, signIn, signOut }),
    [token, signIn, signOut],
  )
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): Auth {
  const auth = useContext(AuthContext)
  if (!auth) {
    throw new Error("useAuth must be used within an AuthProvider")
  }
  return auth
}
