import { readFileSync } from 'node:fs';
import { fileURLToPath, URL } from 'node:url';
import { defineConfig, loadEnv, normalizePath, type Plugin } from 'vite';
import ts from 'typescript';

const web = fileURLToPath(new URL('./web/', import.meta.url));
const projectRoot = fileURLToPath(new URL('./', import.meta.url));

// Run before first paint while keeping CSP free of inline scripts.
function themeScript(): Plugin {
  const source = fileURLToPath(new URL('./web/theme.ts', import.meta.url));
  const compile = () => ts.transpileModule(readFileSync(source, 'utf8'), {
    compilerOptions: { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.None },
  }).outputText;
  return {
    name: 'sync-theme',
    configureServer(server) {
      server.watcher.add(source);
      server.watcher.on('change', (file) => {
        if (normalizePath(file) === normalizePath(source)) server.ws.send({ type: 'full-reload' });
      });
      server.middlewares.use('/theme.js', (_req, res) => {
        res.setHeader('Content-Type', 'text/javascript; charset=utf-8');
        res.setHeader('Cache-Control', 'no-cache');
        res.end(compile());
      });
    },
    generateBundle() {
      this.emitFile({ type: 'asset', fileName: 'theme.js', source: compile() });
    },
  };
}

export default defineConfig(({ mode }) => {
  // Match Go's project-level .env and allow an explicit development proxy override.
  const env = loadEnv(mode, projectRoot, ['VITE_API_TARGET', 'PORT']);
  const apiTarget = env.VITE_API_TARGET?.trim() || `http://127.0.0.1:${env.PORT?.trim() || '18080'}`;

  return {
    appType: 'spa',
    root: web,
    envDir: projectRoot,
    publicDir: 'static',
    plugins: [themeScript()],
    server: {
      host: '127.0.0.1',
      proxy: {
        '/v1/api': { target: apiTarget, changeOrigin: false },
        '/health': { target: apiTarget, changeOrigin: false },
      },
    },
    build: {
      outDir: 'dist',
      emptyOutDir: true,
    },
  };
});
