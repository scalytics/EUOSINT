import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./mobile.css";
import { MobileApp } from "./MobileApp";
import { ErrorBoundary } from "@/components/ErrorBoundary";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ErrorBoundary>
      <MobileApp />
    </ErrorBoundary>
  </StrictMode>,
);
