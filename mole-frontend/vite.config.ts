import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  // react-rnd's CommonJS dependency uses this optional debug flag directly.
  // Define it at build time so no Node `process` global is needed in browsers.
  define: {
    'process.env.DRAGGABLE_DEBUG': 'false',
  },
})
