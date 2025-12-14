import { Plugin } from 'vite';
import fs from 'fs';
import path from 'path';

/**
 * Plugin to resolve .js imports from protobuf-es generated files to .ts files
 */
export function protoResolver(): Plugin {
  return {
    name: 'proto-resolver',
    resolveId(source, importer) {
      if (!importer) return null;

      // Only handle relative imports ending in .js
      if (source.endsWith('.js') && (source.startsWith('./') || source.startsWith('../'))) {
        const importerDir = path.dirname(importer);
        const resolvedPath = path.resolve(importerDir, source);

        // Try to find the .ts file
        const tsPath = resolvedPath.replace(/\.js$/, '.ts');
        if (fs.existsSync(tsPath)) {
          return tsPath;
        }
      }

      return null;
    }
  };
}
