"use client";

import { useEffect, useState, use, Suspense } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, ChevronDown, ChevronRight, User, Bot, Wrench, Brain, ScrollText } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { getChatTrace, getChatRuns, type TraceMessage, type RunSummary } from "@/lib/api";
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

// Compact wall-clock duration: 340ms / 1.2s / 13s / 2m 4s. Used on the
// assistant stats line; kept symmetric with formatOffset's vocabulary so
// "+1.2s offset" and "1.2s duration" read together cleanly.
function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  if (ms < 10_000) return `${(ms / 1000).toFixed(1)}s`;
  if (ms < 60_000) return `${Math.round(ms / 1000)}s`;
  const mins = Math.floor(ms / 60_000);
  const secs = Math.round((ms % 60_000) / 1000);
  return `${mins}m ${secs}s`;
}

// Token counts are the headline number on a stats line, so prefer
// precision over rounded approximations — "1,234" beats "1.2k" (which
// also displays for 1499, hiding 22% of the count). Insert thousands
// separators for readability; the only case we abbreviate is 1M+ where
// the exact count rarely matters in the runs list.
function formatTokens(n: number): string {
  if (n < 1_000_000) return n.toLocaleString("en-US");
  return `${(n / 1_000_000).toFixed(2)}M`;
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

// One-line per-turn stats rendered under the Assistant title. Each piece
// is independently optional so older sessions (no metadata at all),
// providers that don't report usage, and partial failures all render
// gracefully — fields just disappear instead of showing "0".
function TurnStats({ md }: { md?: import("@/lib/api").TraceMetadata }) {
  if (!md) return null;
  const parts: React.ReactNode[] = [];
  if (md.model) parts.push(<span key="m" className="font-mono">{md.model}</span>);
  if (typeof md.duration_ms === "number") parts.push(<span key="d">{formatDuration(md.duration_ms)}</span>);
  if (typeof md.tokens_in === "number" || typeof md.tokens_out === "number") {
    const inTok = md.tokens_in ?? 0;
    const outTok = md.tokens_out ?? 0;
    parts.push(
      <span key="t">
        {formatTokens(inTok)} → {formatTokens(outTok)}t
      </span>,
    );
  }
  if (typeof md.cache_read === "number" && md.cache_read > 0) {
    parts.push(<span key="c" className="text-muted-foreground/70">cache {formatTokens(md.cache_read)}</span>);
  }
  if (parts.length === 0) return null;
  // Flat row: [part0, ·, part1, ·, part2]. Single gap-x-2 controls all
  // spacing so dot-separator and content stay visually balanced.
  const interleaved: React.ReactNode[] = [];
  parts.forEach((p, i) => {
    if (i > 0) interleaved.push(<span key={`sep-${i}`} className="text-muted-foreground/40">·</span>);
    interleaved.push(p);
  });
  return (
    <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
      {interleaved}
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
  // Two views off the same route: list (no ?session=) and detail. Same
  // route so deep-links and Back-button navigation stay clean.
  if (!decodedSessionId) return <RunsList agentId={agentId} />;
  return <TraceDetail agentId={agentId} sessionId={decodedSessionId} />;
}

function RunsList({ agentId }: { agentId: string }) {
  const router = useRouter();
  const agentName = useAgentName(agentId);
  const [runs, setRuns] = useState<RunSummary[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    getChatRuns(agentId)
      .then((r) => {
        if (cancelled) return;
        // Newest first — server returns whatever ListWebSessions does
        // (which is filesystem order); imposing the sort here keeps the
        // UI stable across stores that don't promise an order.
        const sorted = [...r].sort((a, b) => (b.updatedAt || 0) - (a.updatedAt || 0));
        setRuns(sorted);
      })
      .catch((e) => {
        if (cancelled) return;
        setError(e instanceof Error ? e.message : "Failed to load runs");
      });
    return () => {
      cancelled = true;
    };
  }, [agentId]);

  return (
    <div className="p-6 max-w-5xl mx-auto">
      <div className="mb-6">
        <h1 className="text-2xl font-semibold tracking-tight">
          Runs · {agentName || "Agent"}
        </h1>
        <p className="text-sm text-muted-foreground mt-1">
          One row per chat session. Click a row to inspect the run.
        </p>
      </div>

      {error && (
        <div className="rounded-lg border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">
          {error}
        </div>
      )}

      {!runs && !error && (
        <div className="space-y-2">
          <Skeleton className="h-12" />
          <Skeleton className="h-12" />
          <Skeleton className="h-12" />
        </div>
      )}

      {runs && runs.length === 0 && (
        <div className="rounded-lg border bg-muted/30 p-6 text-sm text-muted-foreground">
          No chat sessions yet for this agent.
        </div>
      )}

      {runs && runs.length > 0 && (
        <div className="rounded-lg border overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-muted/40 text-xs text-muted-foreground">
              <tr>
                <th className="text-left font-medium px-3 py-2">Title</th>
                <th className="text-right font-medium px-3 py-2 hidden sm:table-cell">Turns</th>
                <th className="text-right font-medium px-3 py-2 hidden sm:table-cell">Tools</th>
                <th className="text-right font-medium px-3 py-2 hidden md:table-cell">Tokens</th>
                <th className="text-right font-medium px-3 py-2 hidden md:table-cell">Duration</th>
                <th className="text-left font-medium px-3 py-2 hidden lg:table-cell">Models</th>
                <th className="text-right font-medium px-3 py-2">Updated</th>
              </tr>
            </thead>
            <tbody>
              {runs.map((r) => {
                const href = `/agents/${agentId}/runs/?session=${encodeURIComponent(r.sessionId)}`;
                return (
                <tr
                  key={r.sessionId}
                  className="border-t hover:bg-muted/30 cursor-pointer"
                  onClick={() => router.push(href)}
                >
                  <td className="px-3 py-2">
                    <Link
                      href={href}
                      className="hover:underline inline-flex items-center gap-2"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <ScrollText className="size-3.5 text-muted-foreground shrink-0" />
                      <span className="truncate">
                        {r.title || r.preview || r.sessionId}
                      </span>
                    </Link>
                  </td>
                  <td className="px-3 py-2 text-right tabular-nums hidden sm:table-cell">{r.turnCount || ""}</td>
                  <td className="px-3 py-2 text-right tabular-nums hidden sm:table-cell">{r.toolCallCount || ""}</td>
                  <td className="px-3 py-2 text-right tabular-nums hidden md:table-cell text-muted-foreground">
                    {r.tokensIn || r.tokensOut ? (
                      <span title={`${r.tokensIn} in · ${r.tokensOut} out`}>
                        {formatTokens((r.tokensIn || 0) + (r.tokensOut || 0))}
                      </span>
                    ) : (
                      ""
                    )}
                  </td>
                  <td className="px-3 py-2 text-right tabular-nums hidden md:table-cell text-muted-foreground">
                    {r.durationMs > 0 ? formatDuration(r.durationMs) : ""}
                  </td>
                  <td className="px-3 py-2 hidden lg:table-cell text-xs text-muted-foreground font-mono truncate max-w-[12rem]">
                    {r.models?.join(", ") || ""}
                  </td>
                  <td className="px-3 py-2 text-right text-xs text-muted-foreground whitespace-nowrap">
                    {r.updatedAt ? formatRelativeShort(r.updatedAt) : ""}
                  </td>
                </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// Short relative time used in the runs table — "5m ago" / "2h ago" /
// "3d ago" / dated. Anchored to the current moment so it re-evaluates
// each render; that's fine, the table doesn't re-render often.
function formatRelativeShort(ms: number): string {
  const diff = Date.now() - ms;
  if (diff < 0) return new Date(ms).toLocaleDateString();
  if (diff < 60_000) return "just now";
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m ago`;
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}h ago`;
  if (diff < 7 * 86_400_000) return `${Math.floor(diff / 86_400_000)}d ago`;
  return new Date(ms).toLocaleDateString();
}

function TraceDetail({ agentId, sessionId: decodedSessionId }: { agentId: string; sessionId: string }) {
  const agentName = useAgentName(agentId);

  const [trace, setTrace] = useState<TraceMessage[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
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
            Run · {agentName || "Agent"}
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
                <TurnStats md={m.metadata} />
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
