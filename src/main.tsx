/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import App from "./App.tsx";
import { ErrorBoundary } from "@/components/ErrorBoundary";

// Redirect mobile devices to the dedicated mobile app
if (
  /Android|iPhone|iPod/.test(navigator.userAgent) &&
  window.innerWidth < 768 &&
  !new URLSearchParams(location.search).has("desktop") &&
  !document.cookie.includes("euosint_prefer_desktop")
) {
  location.replace("/m/");
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ErrorBoundary>
      <App />
    </ErrorBoundary>
  </StrictMode>
);
