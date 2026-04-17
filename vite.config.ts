/*
 * kafSIEM
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";
import type { IncomingMessage, ServerResponse } from "http";

function getTagVersionFromRef(ref?: string): string | undefined {
  if (!ref) return undefined;
  const normalized = ref.replace(/^refs\/tags\//, "");
  if (!/^v?\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/.test(normalized)) return undefined;
  return normalized.replace(/^v/, "");
}

function writeJSON(res: ServerResponse<IncomingMessage>, status: number, payload: unknown) {
  res.statusCode = status;
  res.setHeader("Content-Type", "application/json; charset=utf-8");
  res.end(JSON.stringify(payload));
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
  plugins: [
    react(),
    tailwindcss(),
    {
      name: "mobile-spa-fallback",
      configureServer(server) {
        server.middlewares.use((req, _res, next) => {
          if (req.url && /^\/mobile(\/(?!index\.html).*)?\/?(\?.*)?$/.test(req.url)) {
            req.url = "/mobile/index.html";
          }
          next();
        });
        server.middlewares.use((req, res, next) => {
          if (req.url === "/api/demo/agentops/replay" && req.method === "POST") {
            writeJSON(res, 202, {
              status: "accepted",
              mode: "demo",
            });
            return;
          }
          next();
        });
      },
    },
  ],
  build: {
    rollupOptions: {
      input: {
        main: path.resolve(__dirname, "index.html"),
        mobile: path.resolve(__dirname, "mobile/index.html"),
      },
      output: {
        manualChunks(id) {
          if (!id.includes("node_modules")) return undefined;
          if (id.includes("three")) return "vendor-three";
          if (id.includes("leaflet")) return "vendor-leaflet";
          if (id.includes("react") || id.includes("scheduler")) return "vendor-react";
          if (id.includes("lucide-react")) return "vendor-icons";
          return "vendor";
        },
      },
    },
  },
  define: {
    __APP_VERSION__: JSON.stringify(appVersion),
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
