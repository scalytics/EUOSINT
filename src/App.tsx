/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import { useCallback, useEffect, useRef, useState } from "react";
import { Header } from "@/components/Header";
import { StatsBar } from "@/components/StatsBar";
import { GlobeView } from "@/components/GlobeView";
import { AlertFeed } from "@/components/AlertFeed";
import { AlertDetail } from "@/components/AlertDetail";
import { SubmitIntelModal } from "@/components/SubmitIntelModal";
import { useAlerts } from "@/hooks/useAlerts";

export default function App() {
  const { alerts, isLoading, sourceCount } = useAlerts();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [regionFilter, setRegionFilter] = useState<string>("all");
  const [visibleAlertIds, setVisibleAlertIds] = useState<string[]>([]);
  const [mobilePane, setMobilePane] = useState<"map" | "stack">("map");
  const [isDesktopFeedOpen, setIsDesktopFeedOpen] = useState(true);
  const [feedVisible, setFeedVisible] = useState(true);
  const [feedToggled, setFeedToggled] = useState(false);
  const [isSubmitOpen, setIsSubmitOpen] = useState(false);
  const panelRef = useRef<HTMLDivElement>(null);
  const feedRef = useRef<HTMLDivElement>(null);
  const selectedAlert = selectedId
    ? alerts.find((a) => a.alert_id === selectedId) ?? null
    : null;

  const handleClose = useCallback(() => {
    const el = panelRef.current;
    if (!el) { setSelectedId(null); return; }
    el.style.animation = "slide-out-right 0.3s ease-in forwards";
    el.addEventListener("animationend", () => {
      setSelectedId(null);
    }, { once: true });
  }, []);

  const handleHideFeed = useCallback(() => {
    setFeedToggled(true);
    setFeedVisible(false);
    const el = feedRef.current;
    if (!el) { setIsDesktopFeedOpen(false); return; }
    el.addEventListener("animationend", () => {
      setIsDesktopFeedOpen(false);
    }, { once: true });
  }, []);

  const handleShowFeed = useCallback(() => {
    setFeedToggled(true);
    setIsDesktopFeedOpen(true);
    setFeedVisible(true);
  }, []);

  useEffect(() => {
    if (selectedId && !alerts.some((a) => a.alert_id === selectedId)) {
      setSelectedId(null);
    }
  }, [alerts, selectedId]);

  useEffect(() => {
    setVisibleAlertIds(alerts.map((a) => a.alert_id));
  }, [alerts]);

  return (
    <div className="flex flex-col h-[100dvh] bg-siem-bg">
      {/* Top Bar */}
      <Header regionFilter={regionFilter} onSubmitIntel={() => setIsSubmitOpen(true)} />
      <StatsBar alerts={alerts} />
      <div className="md:hidden px-3 py-2 bg-siem-panel border-b border-siem-border">
        <div className="grid grid-cols-2 rounded-md border border-siem-border overflow-hidden">
          <button
            type="button"
            onClick={() => setMobilePane("map")}
            className={`py-1.5 text-[11px] font-mono uppercase tracking-wider transition-colors ${
              mobilePane === "map"
                ? "bg-siem-accent/18 text-siem-accent"
                : "bg-white/5 text-siem-muted"
            }`}
          >
            Globe
          </button>
          <button
            type="button"
            onClick={() => setMobilePane("stack")}
            className={`py-1.5 text-[11px] font-mono uppercase tracking-wider transition-colors ${
              mobilePane === "stack"
                ? "bg-siem-accent/18 text-siem-accent"
                : "bg-white/5 text-siem-muted"
            }`}
          >
            Alert Stack
          </button>
        </div>
      </div>

      {/* Main Content */}
      <div className="flex flex-1 min-h-0 relative">
        {!isDesktopFeedOpen && (
          <button
            type="button"
            onClick={handleShowFeed}
            className="hidden md:flex absolute left-2 top-1/2 -translate-y-1/2 z-10 items-center gap-1 rounded-md border border-siem-border bg-siem-panel/85 px-2 py-1.5 text-[10px] font-mono uppercase tracking-wider text-siem-text hover:bg-siem-accent/12 hover:text-siem-accent transition-colors"
          >
            &#8250; Stack
          </button>
        )}
        {/* Left Panel: Alert Feed */}
        {(isDesktopFeedOpen || mobilePane === "stack") && (
        <div
          ref={feedRef}
          className={`${
            mobilePane === "stack" ? "flex" : "hidden"
          } md:flex w-full md:w-[340px] md:min-w-[300px] md:border-r md:border-siem-border bg-siem-panel flex-col min-h-0 overflow-hidden relative${
            feedToggled ? (feedVisible ? " feed-panel-open" : " feed-panel-closed") : ""
          }`}
        >
          {isLoading ? (
            <div className="flex-1 flex items-center justify-center text-siem-muted text-sm">
              Loading live feed...
            </div>
          ) : (
            <AlertFeed
              alerts={alerts}
              selectedId={selectedId}
              onSelect={setSelectedId}
              regionFilter={regionFilter}
              onRegionChange={setRegionFilter}
              onVisibleAlertIdsChange={setVisibleAlertIds}
              onHideDesktop={handleHideFeed}
            />
          )}
        </div>
        )}

        {/* Center: Globe (full remaining width) */}
        <div
          className={`${
            mobilePane === "map" ? "block" : "hidden"
          } md:block flex-1 relative bg-siem-bg min-h-0`}
        >
          <GlobeView
            alerts={alerts}
            selectedId={selectedId}
            onSelect={setSelectedId}
            regionFilter={regionFilter}
            onRegionChange={setRegionFilter}
            visibleAlertIds={visibleAlertIds}
          />
        </div>

        {/* Right Panel: Slide-out Alert Detail */}
        {selectedAlert && (
          <div
            ref={panelRef}
            className="absolute inset-0 md:top-0 md:right-0 md:left-auto md:h-full z-20 flex animate-slide-in"
          >
            {/* Backdrop click to close */}
            <div
              className="hidden md:block w-8 cursor-pointer bg-gradient-to-r from-transparent to-black/30"
              onClick={handleClose}
            />
            <div className="w-full md:w-[380px] bg-siem-panel md:border-l border-siem-border flex flex-col shadow-2xl shadow-black/50">
              <AlertDetail
                alert={selectedAlert}
                onClose={handleClose}
              />
            </div>
          </div>
        )}
      </div>

      <SubmitIntelModal isOpen={isSubmitOpen} onClose={() => setIsSubmitOpen(false)} />

      {/* Bottom Status Bar */}
      <div className="flex items-center justify-between px-3 md:px-6 py-1.5 bg-siem-panel border-t border-siem-border text-[10px] text-siem-muted font-mono">
        <span className="truncate">EUOSINT v0.1.0</span>
        <span className="hidden md:inline">
          Sources: {sourceCount} authorities // Live feed // No data stored // Index + Link only
        </span>
        <span className="md:hidden">{sourceCount} src</span>
      </div>
    </div>
  );
}
