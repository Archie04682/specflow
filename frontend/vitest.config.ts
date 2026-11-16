import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

// Tests run in jsdom. The production build is configured in vite.config.ts;
// this file only adds the test environment so the two never drift.
export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    restoreMocks: true,
    css: false,
  },
});
