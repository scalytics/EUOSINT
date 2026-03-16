/*
 * EUOSINT
 * Portions derived from novatechflow/osint-siem and cyberdude88/osint-siem.
 * See NOTICE for provenance and LICENSE for repository-local terms.
 */

import { useState } from "react";
import { X, ExternalLink } from "lucide-react";

interface Props {
  isOpen: boolean;
  onClose: () => void;
}

const REPO_OWNER = "scalytics";
const REPO_NAME = "EUOSINT";

const SEVERITY_OPTIONS = [
  { value: "critical", label: "Critical" },
  { value: "high", label: "High" },
  { value: "medium", label: "Medium" },
  { value: "low", label: "Low" },
  { value: "info", label: "Informational" },
] as const;

const CATEGORY_OPTIONS = [
  { value: "crypto_heist", label: "Crypto Heist / Theft" },
  { value: "data_breach", label: "Data Breach" },
  { value: "corporate_fraud", label: "Corporate Fraud" },
  { value: "ransomware", label: "Ransomware Incident" },
  { value: "supply_chain", label: "Supply Chain Attack" },
  { value: "other", label: "Other Private Sector Incident" },
] as const;

export function SubmitIntelModal({ isOpen, onClose }: Props) {
  const [title, setTitle] = useState("");
  const [sourceUrl, setSourceUrl] = useState("");
  const [category, setCategory] = useState("data_breach");
  const [severity, setSeverity] = useState("high");
  const [description, setDescription] = useState("");
  const [affectedOrg, setAffectedOrg] = useState("");

  if (!isOpen) return null;

  const issueTitle = `[User Intel] ${title}`;

  const issueBody = [
    "## User-Submitted Intelligence Report",
    "",
    `**Incident Type:** ${CATEGORY_OPTIONS.find((c) => c.value === category)?.label ?? category}`,
    `**Suggested Severity:** ${severity}`,
    `**Affected Organization:** ${affectedOrg || "N/A"}`,
    `**Source URL:** ${sourceUrl || "N/A"}`,
    "",
    "## Description",
    description || "No description provided.",
    "",
    "---",
    "_Submitted via the EUOSINT user submission form. Please review and categorize under `private_sector` if appropriate._",
  ].join("\n");

  const githubUrl = new URL(
    `https://github.com/${REPO_OWNER}/${REPO_NAME}/issues/new`
  );
  githubUrl.searchParams.set("title", issueTitle);
  githubUrl.searchParams.set("body", issueBody);
  githubUrl.searchParams.set("labels", "user-submission,private_sector");

  const isValid = title.trim().length > 0;

  const handleSubmit = () => {
    if (!isValid) return;
    window.open(githubUrl.toString(), "_blank", "noopener,noreferrer");
  };

  const handleReset = () => {
    setTitle("");
    setSourceUrl("");
    setCategory("data_breach");
    setSeverity("high");
    setDescription("");
    setAffectedOrg("");
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div
        className="absolute inset-0 bg-black/60 backdrop-blur-sm"
        onClick={onClose}
      />
      <div className="relative w-full max-w-lg mx-4 max-h-[85dvh] overflow-y-auto rounded-xl border border-siem-border bg-siem-panel shadow-2xl shadow-black/50">
        {/* Header */}
        <div className="sticky top-0 z-10 flex items-center justify-between px-5 py-3 border-b border-siem-border bg-siem-panel rounded-t-xl">
          <div>
            <h2 className="text-sm font-bold uppercase tracking-wider text-siem-text">
              Submit Intelligence
            </h2>
            <p className="text-[10px] text-siem-muted uppercase tracking-widest mt-0.5">
              Private Sector Incident Report
            </p>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded hover:bg-siem-accent/12 hover:text-siem-accent text-siem-muted transition-colors"
          >
            <X size={14} />
          </button>
        </div>

        {/* Form */}
        <div className="p-5 space-y-4">
          <div>
            <label className="block text-[10px] uppercase tracking-wider text-siem-muted mb-1.5">
              Incident Title *
            </label>
            <input
              type="text"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="e.g., Exchange X hacked for $50M in crypto"
              className="w-full bg-white/5 border border-siem-border rounded-md px-3 py-2 text-sm text-siem-text placeholder-siem-muted/50 focus:outline-none focus:ring-1 focus:ring-siem-accent"
            />
          </div>

          <div>
            <label className="block text-[10px] uppercase tracking-wider text-siem-muted mb-1.5">
              Source URL
            </label>
            <input
              type="url"
              value={sourceUrl}
              onChange={(e) => setSourceUrl(e.target.value)}
              placeholder="https://..."
              className="w-full bg-white/5 border border-siem-border rounded-md px-3 py-2 text-sm text-siem-text placeholder-siem-muted/50 focus:outline-none focus:ring-1 focus:ring-siem-accent"
            />
          </div>

          <div>
            <label className="block text-[10px] uppercase tracking-wider text-siem-muted mb-1.5">
              Affected Organization
            </label>
            <input
              type="text"
              value={affectedOrg}
              onChange={(e) => setAffectedOrg(e.target.value)}
              placeholder="e.g., Acme Corp, Binance, etc."
              className="w-full bg-white/5 border border-siem-border rounded-md px-3 py-2 text-sm text-siem-text placeholder-siem-muted/50 focus:outline-none focus:ring-1 focus:ring-siem-accent"
            />
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-[10px] uppercase tracking-wider text-siem-muted mb-1.5">
                Incident Type
              </label>
              <select
                value={category}
                onChange={(e) => setCategory(e.target.value)}
                className="w-full appearance-none bg-white/5 border border-siem-border rounded-md px-3 py-2 text-sm text-siem-text focus:outline-none focus:ring-1 focus:ring-siem-accent"
              >
                {CATEGORY_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {opt.label}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-[10px] uppercase tracking-wider text-siem-muted mb-1.5">
                Severity
              </label>
              <select
                value={severity}
                onChange={(e) => setSeverity(e.target.value)}
                className="w-full appearance-none bg-white/5 border border-siem-border rounded-md px-3 py-2 text-sm text-siem-text focus:outline-none focus:ring-1 focus:ring-siem-accent"
              >
                {SEVERITY_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {opt.label}
                  </option>
                ))}
              </select>
            </div>
          </div>

          <div>
            <label className="block text-[10px] uppercase tracking-wider text-siem-muted mb-1.5">
              Description
            </label>
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={4}
              placeholder="Provide details about the incident..."
              className="w-full bg-white/5 border border-siem-border rounded-md px-3 py-2 text-sm text-siem-text placeholder-siem-muted/50 focus:outline-none focus:ring-1 focus:ring-siem-accent resize-none"
            />
          </div>

          <div className="rounded-lg border border-siem-accent/25 bg-siem-accent/8 p-3">
            <p className="text-[11px] text-siem-accent leading-relaxed">
              Clicking &ldquo;Submit via GitHub&rdquo; opens a pre-filled GitHub
              Issue in a new tab. You will need a GitHub account. Submissions are
              reviewed by maintainers before being added to the feed.
            </p>
          </div>
        </div>

        {/* Footer */}
        <div className="sticky bottom-0 flex items-center justify-between px-5 py-3 border-t border-siem-border bg-siem-panel rounded-b-xl">
          <button
            type="button"
            onClick={handleReset}
            className="px-3 py-1.5 text-xs text-siem-muted hover:text-siem-text transition-colors"
          >
            Reset
          </button>
          <button
            type="button"
            onClick={handleSubmit}
            disabled={!isValid}
            className={`flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-bold transition-colors ${
              isValid
                ? "bg-siem-accent hover:bg-siem-accent/80 text-white"
                : "bg-white/5 text-siem-muted cursor-not-allowed"
            }`}
          >
            <ExternalLink size={14} />
            Submit via GitHub
          </button>
        </div>
      </div>
    </div>
  );
}
