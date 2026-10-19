import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

// The console builds into ../web/dist, which is what go:embed ships inside the
// binary. Keeping the output inside the module is what lets `go run` serve the
// built console with no Node on the host (PRD US-8, one command).
export default defineConfig({
  plugins: [react()],
  base: '/',
  build: {
    outDir: '../web/dist',
    emptyOutDir: true,
    sourcemap: false,
    // One chunk per vendor family keeps the map libraries out of the shell's
    // critical path: the sidebar paints before MapLibre and deck.gl arrive.
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules/react') || id.includes('node_modules/react-dom') || id.includes('node_modules/react-router-dom')) {
            return 'react'
          }
          if (id.includes('node_modules/maplibre-gl') || id.includes('node_modules/@deck.gl')) {
            return 'map'
          }
          return undefined
        },
      },
    },
  },
  server: {
    port: 5173,
    // The dev server talks to the Go process, so the console can be developed
    // without rebuilding the binary. `go run ./cmd/fleetflow -demo` on 8080.
    proxy: {
      '/api': { target: 'http://127.0.0.1:8080', changeOrigin: true },
      '/healthz': { target: 'http://127.0.0.1:8080', changeOrigin: true },
      '/readyz': { target: 'http://127.0.0.1:8080', changeOrigin: true },
    },
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
})
