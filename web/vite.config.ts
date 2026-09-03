import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The production build is embedded into the Go binary from web/dist via
// go:embed. During development, `vite dev` proxies API traffic to the Go
// server so the SPA and backend share an origin.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://localhost:8080",
      "/metrics": "http://localhost:8080",
      "/healthz": "http://localhost:8080",
    },
  },
});
