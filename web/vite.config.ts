import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// In development the gateway runs on :8080; Vite proxies the API to it so
// the browser sees one origin, the same as in production.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/graphql": "http://localhost:8080",
    },
  },
});
