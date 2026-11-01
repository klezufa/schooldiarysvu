import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// Адрес backend для dev-прокси можно переопределить, например, чтобы проверить свежую сборку на другом порту.
const apiTarget = process.env.VITE_API_PROXY ?? 'http://localhost:8080'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      '/api': apiTarget,
      '/health': apiTarget,
    },
  },
  build: {
    // Шрифты всегда отдельными файлами: CSP разрешает font-src только 'self', встроенные data:-шрифты браузер заблокирует.
    assetsInlineLimit: (file) => (/\.(woff2?|ttf|otf)$/.test(file) ? false : undefined),
    rollupOptions: {
      output: {
        // Библиотеки — отдельными чанками: они меняются реже приложения и остаются в кеше браузера после обновления.
        manualChunks(id) {
          if (!id.includes('node_modules')) return undefined
          if (/[\\/](react|react-dom|scheduler|react-router|react-router-dom)[\\/]/.test(id)) return 'react'
          if (id.includes('@tanstack')) return 'tanstack'
          if (/[\\/](zod|react-hook-form|@hookform)[\\/]/.test(id)) return 'forms'
          if (id.includes('@radix-ui')) return 'radix'
          return undefined
        },
      },
      // zod содержит аннотации /* @__PURE__ */, которые Rollup не может разместить: это шум зависимости, а не ошибка проекта.
      onwarn(warning, defaultHandler) {
        if (warning.code === 'INVALID_ANNOTATION' && warning.id?.includes('node_modules')) return
        defaultHandler(warning)
      },
    },
  },
  test: {
    include: ['src/**/*.test.{ts,tsx}'],
    globals: true,
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
  },
})
