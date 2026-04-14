import type { ReactNode } from "react";
import { Workflow } from "lucide-react";

export function MetricCard({ icon: Icon, label, value, hint }: { icon: typeof Workflow; label: string; value: string; hint: string }) {
  return (
    <div className="min-w-[150px] rounded-2xl border border-siem-border bg-siem-panel/60 px-4 py-3">
      <div className="flex items-center justify-between gap-3">
        <span className="text-[11px] uppercase tracking-[0.2em] text-siem-muted">{label}</span>
        <Icon size={15} className="text-siem-accent" />
      </div>
      <div className="mt-2 text-2xl font-semibold">{value}</div>
      <div className="mt-1 text-xs text-siem-muted">{hint}</div>
    </div>
  );
}

export function Panel({ title, icon: Icon, children }: { title: string; icon: typeof Workflow; children: ReactNode }) {
  return (
    <section className="rounded-[24px] border border-siem-border bg-siem-panel/80 p-4 shadow-[0_18px_50px_rgba(0,0,0,0.2)]">
      <div className="mb-4 flex items-center gap-2 text-[11px] uppercase tracking-[0.24em] text-siem-muted">
        <Icon size={14} className="text-siem-accent" />
        {title}
      </div>
      {children}
    </section>
  );
}

export function StatusRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-2xl border border-siem-border bg-siem-panel/55 px-3 py-2 text-sm">
      <span className="text-siem-muted">{label}</span>
      <span className="font-semibold">{value || "-"}</span>
    </div>
  );
}

export function Tag({ children }: { children: ReactNode }) {
  return <span className="rounded-full border border-siem-border px-2 py-1">{children}</span>;
}

export function EmptyState({ text }: { text: string }) {
  return <div className="rounded-2xl border border-dashed border-siem-border bg-siem-panel/45 p-4 text-sm text-siem-muted">{text}</div>;
}
