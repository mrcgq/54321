import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

export default defineConfig({
  plugins: [vue()],
  // ⚠️【关键修改】强制使用相对路径，修复白屏问题
  base: './', 
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
      '@wails': resolve(__dirname, 'wailsjs')
    }
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    sourcemap: false,
    minify: 'terser',
    terserOptions: {
      compress: {
        drop_console: true,
        drop_debugger: true
      }
    }
  },
  server: {
    strictPort: true
  }
})
