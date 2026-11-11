import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The API base defaults to a sibling ledgerd on :8080; override with VITE_API_BASE.
// A dev proxy keeps the browser same-origin so no CORS config is needed on the toy API.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: process.env.VITE_API_TARGET ?? "http://localhost:8080",
        changeOrigin: true,
        rewrite: (p) => p.replace(/^\/api/, ""),
      },
    },
  },
});
