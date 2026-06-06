import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:8081',
        changeOrigin: true,
        headers: {
          Referer: 'http://localhost:3000'
        }
      },
      '/ws': {
        target: 'ws://localhost:8081',
        ws: true,
        changeOrigin: true
      },
      '^/(images|files|voices|videos|thumbnails)/': {
        target: 'http://localhost:8081',
        changeOrigin: true
      }
    }
  }
})
