import { defineConfig } from 'vite';
import path from 'path';
import { protoResolver } from './vite-plugin-proto-resolver';

export default defineConfig({
  plugins: [protoResolver()],
  root: 'src',
  build: {
    outDir: '../static/js',
    emptyOutDir: false,
    sourcemap: true, // Enable source maps for debugging
    minify: 'terser',
    terserOptions: {
      compress: {
        drop_console: false, // Keep console logs in production
        drop_debugger: true,
      },
    },
    rollupOptions: {
      input: {
        main: path.resolve(__dirname, 'src/main.ts'),
      },
      output: {
        entryFileNames: 'main.bundle.js',
        chunkFileNames: '[name].js',
        assetFileNames: '[name].[ext]',
        sourcemapExcludeSources: false, // Include source content in sourcemaps
      }
    }
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      '@proto': path.resolve(__dirname, './src/proto')
    },
    extensions: ['.ts', '.js', '.tsx', '.jsx']
  },
  esbuild: {
    // Allow resolving .js imports to .ts files
    // This is needed for protobuf-es generated code
    loader: 'ts'
  },
  optimizeDeps: {
    esbuildOptions: {
      resolveExtensions: ['.ts', '.js']
    }
  }
});
