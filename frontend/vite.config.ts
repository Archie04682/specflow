import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: {
    // Built straight into the directory the Go binary embeds. Never emptied:
    // the committed .gitkeep keeps //go:embed valid before the first build.
    outDir: "../backend/web/dist",
    emptyOutDir: false,
  },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
});
