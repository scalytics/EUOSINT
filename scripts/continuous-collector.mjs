/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import { spawn } from "node:child_process";

const RESTART_DELAY_MIN_MS = Number.parseInt(
  process.env.COLLECTOR_RESTART_DELAY_MIN_MS ?? "5000",
  10
);
const RESTART_DELAY_MAX_MS = Number.parseInt(
  process.env.COLLECTOR_RESTART_DELAY_MAX_MS ?? "60000",
  10
);
const DEFAULT_INTERVAL_MS = process.env.INTERVAL_MS ?? "180000";
const DEFAULT_MAX_PER_SOURCE = process.env.MAX_PER_SOURCE ?? "60";

let stopping = false;
let child = null;
let restartAttempt = 0;

function clamp(value, min, max) {
  return Math.max(min, Math.min(max, value));
}

function computeBackoff(attempt) {
  const raw = RESTART_DELAY_MIN_MS * Math.pow(1.7, attempt);
  return clamp(Math.round(raw), RESTART_DELAY_MIN_MS, RESTART_DELAY_MAX_MS);
}

function launchCollector() {
  const env = {
    ...process.env,
    WATCH: "1",
    INTERVAL_MS: DEFAULT_INTERVAL_MS,
    MAX_PER_SOURCE: DEFAULT_MAX_PER_SOURCE,
    MISSING_PERSON_RELEVANCE_THRESHOLD:
      process.env.MISSING_PERSON_RELEVANCE_THRESHOLD ?? "0",
  };

  console.log(
    `[collector] starting feed watcher (INTERVAL_MS=${env.INTERVAL_MS}, MAX_PER_SOURCE=${env.MAX_PER_SOURCE})`
  );

  child = spawn("node", ["scripts/fetch-alerts.mjs", "--watch"], {
    env,
    stdio: "inherit",
    cwd: process.cwd(),
  });

  child.on("exit", (code, signal) => {
    child = null;
    if (stopping) return;
    const delay = computeBackoff(restartAttempt);
    restartAttempt += 1;
    console.warn(
      `[collector] watcher exited (code=${code ?? "null"}, signal=${signal ?? "null"}); restarting in ${delay}ms`
    );
    setTimeout(() => {
      if (!stopping) launchCollector();
    }, delay);
  });

  child.on("error", (error) => {
    console.error(`[collector] failed to launch watcher: ${error.message}`);
  });

  restartAttempt = 0;
}

function shutdown(signal) {
  if (stopping) return;
  stopping = true;
  console.log(`[collector] stopping due to ${signal}`);
  if (!child) {
    process.exit(0);
    return;
  }
  child.once("exit", () => process.exit(0));
  child.kill("SIGTERM");
  setTimeout(() => {
    if (child) {
      child.kill("SIGKILL");
    }
  }, 4000);
}

process.on("SIGINT", () => shutdown("SIGINT"));
process.on("SIGTERM", () => shutdown("SIGTERM"));

launchCollector();
