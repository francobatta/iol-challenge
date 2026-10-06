/// <reference types="vitest/config" />
import path from "node:path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

// In development /v1 goes to the API, as it does through nginx in the image, so the
// browser only ever talks to one origin. API_URL says where the API is; inside a
// container that is not localhost.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(import.meta.dirname, "./src") } },
  server: { proxy: { "/v1": process.env.API_URL ?? "http://localhost:8080" } },
  test: { environment: "jsdom" },
})
