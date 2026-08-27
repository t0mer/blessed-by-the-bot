/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'node:path'

// The build writes straight into the Go embed directory, so `make build` needs
// no copy step: `go:embed all:dist` in internal/webui picks it up as-is.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': path.resolve(import.meta.dirname, 'src') } },
  build: {
    outDir: path.resolve(import.meta.dirname, '../internal/webui/dist'),
    emptyOutDir: true,
  },
  test: {
    environment: 'jsdom',
    globals: true,
    // Reset vi.fn() call history between tests. Without this, mock.calls[0] in
    // one test can be a call made by an earlier one in the same file — an
    // assertion that reads as passing while checking the wrong thing.
    clearMocks: true,
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
    coverage: {
      provider: 'v8',
      include: ['src/App.tsx', 'src/lib/**', 'src/components/**', 'src/pages/**'],
    },
  },
  server: {
    port: 5173,
    // Dev mode proxies the API to the Go binary so the SPA runs against real data.
    proxy: {
      '/api': 'http://127.0.0.1:8080',
      '/healthz': 'http://127.0.0.1:8080',
    },
  },
})
