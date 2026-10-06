import { Navigate, Route, Routes } from "react-router"

import { AppShell } from "@/components/AppShell"
import { useAuth } from "@/lib/auth"
import Dashboard from "@/pages/Dashboard"
import Docs from "@/pages/Docs"
import ListDetail from "@/pages/ListDetail"
import Lists from "@/pages/Lists"
import Login from "@/pages/Login"
import Notifications from "@/pages/Notifications"
import UserDetail from "@/pages/UserDetail"
import Users from "@/pages/Users"

export default function App() {
  const { token } = useAuth()
  return (
    <Routes>
      {/* The docs are for anyone, signed in or not. */}
      <Route path="docs" element={<Docs />} />
      {token ? (
        <Route element={<AppShell />}>
          <Route index element={<Dashboard />} />
          <Route path="users" element={<Users />} />
          <Route path="users/:userID" element={<UserDetail />} />
          <Route path="lists" element={<Lists />} />
          <Route path="lists/:listID" element={<ListDetail />} />
          <Route path="notifications" element={<Notifications />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      ) : (
        // Without a token every other address is the sign-in page.
        <Route path="*" element={<Login />} />
      )}
    </Routes>
  )
}
