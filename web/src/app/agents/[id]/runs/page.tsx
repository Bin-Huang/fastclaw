"use client";

import { useEffect, useState, use, Suspense } from "react";
import { useSearchParams } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, ChevronDown, ChevronRight, User, Bot, Wrench, Brain } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { getChatTrace, type TraceMessage } from "@/lib/api";
import { useAgentName } from "@/hooks/use-agent-name";

// Render a relative offset like "+340ms" / "+1.2s" / "+13s" / "+2m 4s".
// Anchor is the first message's timestamp; later events are differences
// from that. We deliberately drop sub-second precision once we cross 10s
// because anything finer is noise on a multi-turn run. Negative values
// (which shouldn't happen but can if timestamps got out of order) keep
// their sign so the data oddity is visible rather than masked.
function formatOffset(ms: number): string {
  if (ms === 0) return "+0ms";
  const sign = ms > 0 ? "+" : "-";
  const abs = Math.abs(ms);
  if (abs < 1000) return `${sign}${abs}ms`;
  if (abs < 10_000) return `${sign}${(abs / 1000).toFixed(1)}s`;
  if (abs < 60_000) return `${sign}${Math.round(abs / 1000)}s`;
  const mins = Math.floor(abs / 60_000);
  const secs = Math.round((abs % 60_000) / 1000);
  return `${sign}${mins}m ${secs}s`;
}

function formatAbsolute(ms?: number): string {
  if (!ms) return "";
  const d = new Date(ms);
  return d.toLocaleString();
}

function CollapsibleBlock({ label, body }: { label: string; body: string }) {
  const [open, setOpen] = useState(false);
  if (!body) return null;
  return (
    <div className="text-xs">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="inline-flex items-center gap-1 text-muted-foreground hover:text-foreground"
      >
        {open ? <ChevronDown className="size-3" /> : <ChevronRight className="size-3" />}
        {label}
      </button>
      {open && (
        <pre className="mt-1 overflow-x-auto rounded-md border bg-muted/40 p-2 font-mono whitespace-pre-wrap break-words">
          {body}
        </pre>
      )}
    </div>
  );
}

interface RowShellProps {
  icon: React.ReactNode;
  iconBg: string;
  title: React.ReactNode;
  offset: string;
  absolute: string;
  children?: React.ReactNode;
}

function RowShell({ icon, iconBg, title, offset, absolute, children }: RowShellProps) {
  return (
    <div className="relative pl-10">
      <div
        className={`absolute left-2 top-1 flex size-6 items-center justify-center rounded-full ${iconBg} text-white`}
      >
        {icon}
      </div>
      <div className="flex items-baseline gap-2">
        <div className="text-sm font-medium">{title}</div>
        <div className="text-xs text-muted-foreground" title={absolute}>
          {offset}
        </div>
      </div>
      {children && <div className="mt-2 space-y-2">{children}</div>}
    </div>
  );
}

interface PageProps {
  params: Promise<{ id: string }>;
}

export default function TracePage(props: PageProps) {
  return (
    <Suspense fallback={<div className="p-6"><Skeleton className="h-20" /></div>}>
      <TracePageInner {...props} />
    </Suspense>
  );
}

function TracePageInner({ params }: PageProps) {
  const { id: agentId } = use(params);
  const searchParams = useSearchParams();
  const decodedSessionId = searchParams.get("session") || "";
  const agentName = useAgentName(agentId);

  const [trace, setTrace] = useState<TraceMessage[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!decodedSessionId) {
      setTrace([]);
      return;
    }
    let cancelled = false;
    getChatTrace(agentId, decodedSessionId)
      .then((t) => {
        if (cancelled) return;
        setTrace(t);
      })
      .catch((e) => {
        if (cancelled) return;
        setError(e instanceof Error ? e.message : "Failed to load trace");
      });
    return () => {
      cancelled = true;
    };
  }, [agentId, decodedSessionId]);

  // Anchor offsets to the first message that actually has a timestamp —
  // older sessions wrote messages without one, so we fall back to relative
  // index-only labelling in that case.
  const anchorTs = trace?.find((m) => m.timestamp && m.timestamp > 0)?.timestamp ?? 0;

  // Pair tool-role messages with the assistant's tool_call by id so the
  // result lands inside the same row instead of dangling below as a
  // detached "tool" event the way the raw JSONL reads. We also track
  // which tool_call ids an assistant message actually claims, so tool
  // results whose parent is missing (truncated session, edited history,
  // legacy data) can still surface as standalone rows instead of
  // silently disappearing — that's worse than the chat view.
  const resolvedToolResults = new Map<string, { name?: string; content: string; ts?: number }>();
  const claimedToolCallIds = new Set<string>();
  if (trace) {
    for (const m of trace) {
      if (m.role === "assistant" && m.toolCalls) {
        for (const tc of m.toolCalls) claimedToolCallIds.add(tc.id);
      }
      if (m.role === "tool" && m.toolCallId) {
        resolvedToolResults.set(m.toolCallId, {
          name: m.name,
          content: m.content || "",
          ts: m.timestamp,
        });
      }
    }
  }

  return (
    <div className="p-6 max-w-4xl mx-auto">
      <div className="mb-6 flex items-center justify-between gap-4">
        <div className="min-w-0">
          <Link
            href={`/agents/${agentId}/chat/?session=${encodeURIComponent(decodedSessionId)}`}
            className="text-xs text-muted-foreground hover:text-foreground inline-flex items-center gap-1"
          >
            <ArrowLeft className="size-3" />
            Back to chat
          </Link>
          <h1 className="mt-1 text-2xl font-semibold tracking-tight truncate">
            Trace · {agentName || "Agent"}
          </h1>
          <p className="text-xs text-muted-foreground font-mono mt-1 truncate">
            {decodedSessionId}
          </p>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">
          {error}
        </div>
      )}

      {!trace && !error && (
        <div className="space-y-4">
          <Skeleton className="h-20" />
          <Skeleton className="h-20" />
          <Skeleton className="h-20" />
        </div>
      )}

      {trace && trace.length === 0 && (
        <div className="rounded-lg border bg-muted/30 p-6 text-sm text-muted-foreground">
          No messages in this session yet.
        </div>
      )}

      {trace && trace.length > 0 && (
        <div className="relative space-y-6 border-l border-border ml-4 pl-0">
          {trace.map((m, idx) => {
            // Skip tool-role messages whose parent assistant is in the
            // trace — they render nested inside that row. Orphans (no
            // claiming parent) fall through and render as standalone
            // rows so corrupt / truncated sessions don't lose data.
            if (m.role === "tool" && m.toolCallId && claimedToolCallIds.has(m.toolCallId)) {
              return null;
            }

            const ts = m.timestamp || 0;
            const offset = anchorTs && ts ? formatOffset(ts - anchorTs) : `#${idx + 1}`;
            const absolute = formatAbsolute(ts);

            if (m.role === "tool") {
              return (
                <RowShell
                  key={idx}
                  icon={<Wrench className="size-3.5" />}
                  iconBg="bg-amber-500"
                  title={
                    <span className="flex items-center gap-2">
                      Tool result
                      <Badge variant="outline" className="text-[10px]">
                        orphan
                      </Badge>
                    </span>
                  }
                  offset={offset}
                  absolute={absolute}
                >
                  <div className="rounded-md border bg-muted/20 p-3 space-y-1">
                    <div className="text-xs text-muted-foreground font-mono">
                      {m.name || m.toolCallId || "(unknown tool)"}
                    </div>
                    <CollapsibleBlock label="result" body={m.content || ""} />
                  </div>
                </RowShell>
              );
            }

            if (m.role === "user") {
              return (
                <RowShell
                  key={idx}
                  icon={<User className="size-3.5" />}
                  iconBg="bg-blue-500"
                  title="User"
                  offset={offset}
                  absolute={absolute}
                >
                  <div className="rounded-md border bg-card p-3 text-sm whitespace-pre-wrap break-words">
                    {m.content || <span className="text-muted-foreground italic">(no text)</span>}
                  </div>
                  {m.imageUrls && m.imageUrls.length > 0 && (
                    <div className="text-xs text-muted-foreground">
                      {m.imageUrls.length} image attachment{m.imageUrls.length > 1 ? "s" : ""}
                    </div>
                  )}
                </RowShell>
              );
            }

            // assistant
            const hasToolCalls = m.toolCalls && m.toolCalls.length > 0;
            return (
              <RowShell
                key={idx}
                icon={hasToolCalls ? <Wrench className="size-3.5" /> : <Bot className="size-3.5" />}
                iconBg={hasToolCalls ? "bg-amber-500" : "bg-emerald-600"}
                title={
                  <span className="flex items-center gap-2">
                    Assistant
                    {hasToolCalls && (
                      <Badge variant="secondary" className="text-[10px]">
                        {m.toolCalls!.length} tool call{m.toolCalls!.length > 1 ? "s" : ""}
                      </Badge>
                    )}
                  </span>
                }
                offset={offset}
                absolute={absolute}
              >
                {m.thinking && (
                  <div className="rounded-md border border-dashed bg-muted/30 p-2">
                    <div className="mb-1 flex items-center gap-1 text-xs text-muted-foreground">
                      <Brain className="size-3" />
                      Thinking
                    </div>
                    <CollapsibleBlock label="show reasoning" body={m.thinking} />
                  </div>
                )}
                {m.content && (
                  <div className="rounded-md border bg-card p-3 text-sm whitespace-pre-wrap break-words">
                    {m.content}
                  </div>
                )}
                {hasToolCalls &&
                  m.toolCalls!.map((tc) => {
                    const result = resolvedToolResults.get(tc.id);
                    return (
                      <div key={tc.id} className="rounded-md border bg-muted/20 p-3 space-y-1">
                        <div className="flex items-center gap-2 text-sm">
                          <Wrench className="size-3.5 text-amber-600" />
                          <span className="font-mono">{tc.name}</span>
                          <span className="text-xs text-muted-foreground font-mono">{tc.id}</span>
                        </div>
                        <CollapsibleBlock label="arguments" body={tc.arguments} />
                        {result ? (
                          <CollapsibleBlock label="result" body={result.content} />
                        ) : (
                          <div className="text-xs text-muted-foreground italic">
                            (no result recorded)
                          </div>
                        )}
                      </div>
                    );
                  })}
              </RowShell>
            );
          })}
        </div>
      )}
    </div>
  );
}
