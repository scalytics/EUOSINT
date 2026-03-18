/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

function getTagVersionFromRef(ref?: string): string | undefined {
  if (!ref) return undefined;
  const normalized = ref.replace(/^refs\/tags\//, "");
  if (!/^v?\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/.test(normalized)) return undefined;
  return normalized.replace(/^v/, "");
}

const appVersion =
  process.env.APP_VERSION ??
  process.env.VITE_APP_VERSION ??
  getTagVersionFromRef(process.env.GITHUB_REF_NAME) ??
  getTagVersionFromRef(process.env.GITHUB_REF) ??
  process.env.npm_package_version ??
  "dev";

export default defineConfig({
  base: process.env.BASE_PATH ?? "/",
  plugins: [react(), tailwindcss()],
  define: {
    __APP_VERSION__: JSON.stringify(appVersion),
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
});
