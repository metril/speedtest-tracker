/// <reference types="vitest/config" />
import { writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { defineConfig, type Plugin } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

// go:embed requires internal/web/dist to exist even when the Go binary is
// built before `npm run build` has produced any assets. Vite's
// `emptyOutDir: true` wipes that directory (and the tracked `.gitkeep`
// placeholder in it) on every build, so restore the placeholder once the
// bundle is written.
function restoreGitkeep(): Plugin {
  return {
    name: 'restore-dist-gitkeep',
    closeBundle() {
      writeFileSync(resolve(import.meta.dirname, '../internal/web/dist/.gitkeep'), '');
    },
  };
}

export default defineConfig({
  plugins: [react(), tailwindcss(), restoreGitkeep()],
  build: {
    // The Go binary embeds internal/web/dist via go:embed.
    outDir: '../internal/web/dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
      '/healthz': 'http://localhost:8080',
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test-setup.ts'],
  },
});
