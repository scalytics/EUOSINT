import type { AgentOpsMessage } from "@/agentops/types";
import { Tag } from "@/agentops/components/Chrome";
import { formatTime } from "@/agentops/lib/view";

export function MessageCard({ message }: { message: AgentOpsMessage }) {
  return (
    <article className="rounded-2xl border border-siem-border bg-siem-panel/55 p-4">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <div className="font-mono text-xs text-siem-text">{message.topic}</div>
          <div className="mt-1 text-[11px] text-siem-muted">
            {formatTime(message.timestamp)} · p{message.partition} · o{message.offset}
          </div>
        </div>
        <div className="flex flex-wrap gap-2 text-[11px] text-siem-muted">
          <Tag>{message.topic_family}</Tag>
          {message.status ? <Tag>{message.status}</Tag> : null}
          {message.sender_id ? <Tag>{message.sender_id}</Tag> : null}
        </div>
      </div>
      <div className="mt-3 rounded-2xl bg-siem-bg/55 p-3 text-sm leading-6 text-siem-text">
        {message.lfs ? (
          <div className="space-y-2">
            <div className="text-xs uppercase tracking-[0.18em] text-siem-muted">LFS-backed payload</div>
            <div className="font-mono text-xs">{message.lfs.path}</div>
            <div className="flex flex-wrap gap-2 text-[11px] text-siem-muted">
              <Tag>{message.lfs.size} bytes</Tag>
              {message.lfs.content_type ? <Tag>{message.lfs.content_type}</Tag> : null}
              {message.lfs.proxy_id ? <Tag>{message.lfs.proxy_id}</Tag> : null}
            </div>
          </div>
        ) : (
          <pre className="m-0 whitespace-pre-wrap break-words font-[inherit] text-sm">{message.content || message.preview || "No content"}</pre>
        )}
      </div>
    </article>
  );
}
