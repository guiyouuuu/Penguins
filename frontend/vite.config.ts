import { defineConfig } from 'vite';

export default defineConfig({
  server: {
    port: 5173,
    // 开发时把 /ws 代理到 Go 后端
    proxy: {
      '/ws': {
        target: 'ws://localhost:8080',
        ws: true,
      },
      '/api': {
        target: 'http://localhost:8080',
      },
    },
  },
});
