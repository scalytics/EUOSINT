/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import { Shield, Globe, Send } from "lucide-react";

interface Props {
  regionFilter: string;
  onSubmitIntel: () => void;
}

export function Header({ regionFilter, onSubmitIntel }: Props) {
  return (
    <div className="flex items-center justify-between px-3 md:px-6 py-2 md:py-2.5 bg-siem-panel border-b border-siem-border">
      <div className="flex items-center gap-2 md:gap-3 min-w-0">
        <div className="flex items-center justify-center w-7 h-7 md:w-8 md:h-8 rounded-lg bg-siem-accent/20 border border-siem-accent/30 shrink-0">
          <Shield size={16} className="text-siem-accent" />
        </div>
        <div className="min-w-0">
          <h1 className="text-sm font-bold tracking-wide truncate">EUOSINT</h1>
          <p className="hidden sm:block text-[10px] text-siem-muted uppercase tracking-widest">
            EU-Focused Authority Bulletin Intelligence
          </p>
        </div>
      </div>
      <div className="flex items-center gap-2 md:gap-4 shrink-0">
        <button
          type="button"
          onClick={onSubmitIntel}
          className="flex items-center gap-1.5 px-2.5 py-1 rounded-md border border-siem-accent/30 bg-siem-accent/12 text-[10px] md:text-xs text-siem-accent font-mono uppercase tracking-wider hover:bg-siem-accent/20 transition-colors"
        >
          <Send size={11} />
          <span className="hidden sm:inline">Submit Intel</span>
          <span className="sm:hidden">Intel</span>
        </button>
        <div className="hidden sm:flex items-center gap-1.5 text-xs text-siem-muted">
          <Globe size={12} />
          <span className="font-mono uppercase">
            {regionFilter === "all" ? "ALL REGIONS" : regionFilter}
          </span>
        </div>
        <div className="flex items-center gap-1.5">
          <div className="w-1.5 h-1.5 rounded-full bg-green-400 animate-pulse" />
          <span className="text-[10px] text-green-400 font-mono uppercase hidden sm:inline">
            Monitoring
          </span>
          <span className="text-[10px] text-green-400 font-mono uppercase sm:hidden">
            Live
          </span>
        </div>
      </div>
    </div>
  );
}
