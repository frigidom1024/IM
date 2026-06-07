import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  build: {
    lib: {
      entry: 'core/index.js',
      name: 'GoChatCore',
      fileName: 'gochat-core'
    },
    rollupOptions: {
      external: ['vue', 'pinia', 'axios'],
      output: {
        globals: {
          vue: 'Vue',
          pinia: 'Pinia',
          axios: 'axios'
        }
      }
    },
    outDir: 'core/dist'
  }
})
