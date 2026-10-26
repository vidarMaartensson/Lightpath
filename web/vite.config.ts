import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// In development the gateway runs on :8080; Vite proxies the API to it so
// the browser sees one origin, the same as in production. The end-to-end
// tests run their own gateway on another port and point the proxy there.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/graphql": process.env.GATEWAY_URL ?? "http://localhost:8080",
    },
  },
});
