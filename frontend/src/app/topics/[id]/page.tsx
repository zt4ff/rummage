"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { fetchTopic, fetchTopicRuns, fetchRunDetail, runTopicNow, deleteTopic, type Topic, type Run, type RunDetail } from "@/lib/api";
import { cronToFriendly } from "@/lib/cron";

function StatusBadge({ status }: { status: string }) {
  const variants: Record<string, string> = {
    completed: "bg-fern text-white",
    running: "bg-ochre text-white",
    failed: "bg-berry text-white",
    pending: "bg-stone text-white",
  };
  return (
    <Badge className={`${variants[status] || "bg-stone text-white"} text-xs font-medium`}>
      {status}
    </Badge>
  );
}

function AgentThreads({ jobCount, merged }: { jobCount: number; merged: boolean }) {
  const width = 400;
  const height = 80;
  const agentY = 15;
  const mergeY = height - 15;

  return (
    <svg
      viewBox={`0 0 ${width} ${height}`}
      className={`w-full h-16 ${merged ? "" : "agent-thread-pulse"}`}
      aria-hidden="true"
    >
      {Array.from({ length: jobCount }).map((_, i) => {
        const x = ((i + 1) * width) / (jobCount + 1);
        const mergeX = width / 2;
        return (
          <line
            key={i}
            x1={x}
            y1={agentY}
            x2={mergeX}
            y2={mergeY}
            stroke="#4A7C59"
            strokeWidth="2"
            className="agent-thread"
            style={{ animationDelay: `${i * 0.3}s` }}
          />
        );
      })}
      <circle
        cx={width / 2}
        cy={mergeY}
        r={merged ? 5 : 3}
        fill={merged ? "#4A7C59" : "#C4922A"}
        className={merged ? "merge-glow" : ""}
      />
    </svg>
  );
}

interface Finding {
  date?: string | null;
  title?: string;
  venue?: string | null;
  sources?: string[];
  summary?: string;
}

function FindingCard({ finding, index }: { finding: Finding; index: number }) {
  return (
    <Card className="result-card">
      <CardContent className="p-5">
        <div className="flex items-start justify-between gap-3 mb-2">
          <h4 className="font-heading text-base text-ink leading-snug">
            {finding.title || "Untitled finding"}
          </h4>
          {finding.date && (
            <span className="text-xs text-stone whitespace-nowrap flex items-center gap-1">
              <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M6.75 3v2.25M17.25 3v2.25M3 18.75V7.5a2.25 2.25 0 012.25-2.25h13.5A2.25 2.25 0 0121 7.5v11.25m-18 0A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75m-18 0v-7.5A2.25 2.25 0 015.25 9h13.5A2.25 2.25 0 0121 11.25v7.5" />
              </svg>
              {finding.date}
            </span>
          )}
        </div>

        {finding.venue && (
          <p className="text-xs text-stone mb-2 flex items-center gap-1">
            <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M15 10.5a3 3 0 11-6 0 3 3 0 016 0z" />
              <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 10.5c0 7.142-7.5 11.25-7.5 11.25S4.5 17.642 4.5 10.5a7.5 7.5 0 1115 0z" />
            </svg>
            {finding.venue}
          </p>
        )}

        {finding.summary && (
          <p className="text-sm text-stone leading-relaxed mb-3">
            {finding.summary}
          </p>
        )}

        {finding.sources && finding.sources.length > 0 && (
          <div className="flex flex-wrap gap-1.5">
            {finding.sources.map((src, i) => (
              <a
                key={i}
                href={src}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1 text-xs text-fern hover:text-fern/80 bg-fern/5 px-2 py-1 rounded-full transition-colors"
              >
                <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M13.19 8.688a4.5 4.5 0 011.242 7.244l-4.5 4.5a4.5 4.5 0 01-6.364-6.364l1.757-1.757m9.86-2.648a4.5 4.5 0 00-1.242-7.244l-4.5-4.5a4.5 4.5 0 00-6.364 6.364L4.34 8.374" />
                </svg>
                {new URL(src).hostname.replace("www.", "")}
              </a>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function MergedResults({ output }: { output: unknown }) {
  if (!output) return null;

  let findings: Finding[] = [];
  if (Array.isArray(output)) {
    findings = output as Finding[];
  } else if (typeof output === "object" && output !== null) {
    const obj = output as Record<string, unknown>;
    if (Array.isArray(obj.findings)) {
      findings = obj.findings as Finding[];
    } else if (Array.isArray(obj.merged_output)) {
      findings = obj.merged_output as Finding[];
    }
  }

  if (findings.length === 0) {
    return (
      <Card className="bg-paper border-fern/20">
        <CardContent className="p-5">
          <pre className="text-xs text-ink overflow-x-auto whitespace-pre-wrap max-h-64 overflow-y-auto font-mono">
            {JSON.stringify(output, null, 2)}
          </pre>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2 mb-4">
        <div className="w-2 h-2 rounded-full bg-fern" />
        <h3 className="font-heading text-lg text-ink">
          {findings.length} finding{findings.length === 1 ? "" : "s"} merged
        </h3>
      </div>
      {findings.map((finding, i) => (
        <FindingCard key={i} finding={finding} index={i} />
      ))}
    </div>
  );
}

function AgentResult({ job }: { job: { role: string; status: string; result?: unknown } }) {
  const findings = Array.isArray(job.result) ? job.result as Finding[] : [];

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <span className="text-xs text-stone">
          {job.role === "general_search" ? "Open Search" : "Source-Specific"}
        </span>
        <StatusBadge status={job.status} />
      </div>
      {findings.length > 0 ? (
        <div className="text-xs text-stone">
          {findings.length} finding{findings.length === 1 ? "" : "s"} extracted
        </div>
      ) : job.result ? (
        <div className="text-xs text-stone italic">Processing results...</div>
      ) : (
        <div className="text-xs text-stone italic">No result yet</div>
      )}
    </div>
  );
}

function RunTimeline({ runs, onRefresh }: { runs: Run[]; onRefresh: () => void }) {
  const [expandedRun, setExpandedRun] = useState<string | null>(null);
  const [runDetails, setRunDetails] = useState<Record<string, RunDetail>>({});
  const pollingRunRef = useRef<string | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const loadDetail = useCallback(async (runId: string) => {
    try {
      const detail = await fetchRunDetail(runId);
      setRunDetails((prev) => ({ ...prev, [runId]: detail }));
      return detail;
    } catch {
      return null;
    }
  }, []);

  useEffect(() => {
    const activeRun = runs.find((r) => r.status === "pending" || r.status === "running");
    if (!activeRun) {
      if (pollRef.current) clearInterval(pollRef.current);
      return;
    }

    pollingRunRef.current = activeRun.id;
    pollRef.current = setInterval(async () => {
      const detail = await loadDetail(activeRun.id);
      if (detail && (detail.run.status === "completed" || detail.run.status === "failed")) {
        if (pollRef.current) clearInterval(pollRef.current);
        pollingRunRef.current = null;
        onRefresh();
      }
    }, 3000);

    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, [runs, loadDetail, onRefresh]);

  const handleExpand = async (runId: string) => {
    if (expandedRun === runId) {
      setExpandedRun(null);
      return;
    }
    setExpandedRun(runId);
    if (!runDetails[runId]) {
      await loadDetail(runId);
    }
  };

  const formatDate = (s: string) => {
    const d = new Date(s);
    return d.toLocaleDateString("en-US", {
      month: "short",
      day: "numeric",
      year: "numeric",
      hour: "numeric",
      minute: "2-digit",
    });
  };

  return (
    <div className="space-y-3">
      {runs.some((r) => r.status === "pending" || r.status === "running") && (
        <p className="text-sm text-ochre animate-pulse flex items-center gap-2">
          <span className="w-2 h-2 rounded-full bg-ochre animate-pulse" />
          A run is in progress, updates appear automatically...
        </p>
      )}
      {runs.map((run) => {
        const detail = runDetails[run.id];
        const isExpanded = expandedRun === run.id;

        return (
          <div key={run.id} className="relative pl-8">
            <div className="absolute left-3 top-0 bottom-0 w-px bg-border" />
            <div
              className={`absolute left-1.5 top-3 w-3 h-3 rounded-full border-2 ${
                run.status === "completed"
                  ? "bg-fern border-fern"
                  : run.status === "running"
                    ? "bg-ochre border-ochre animate-pulse"
                    : run.status === "failed"
                      ? "bg-berry border-berry"
                      : "bg-paper border-stone"
              }`}
            />

            <div className="pb-2">
              <button
                onClick={() => handleExpand(run.id)}
                className="text-left w-full group"
              >
                <div className="flex items-center gap-3">
                  <span className="text-sm text-stone font-medium">
                    {formatDate(run.scheduled_for)}
                  </span>
                  <StatusBadge status={run.status} />
                </div>
              </button>

              {isExpanded && (
                <div className="mt-4 space-y-4">
                  {detail ? (
                    <>
                      {detail.jobs.length > 1 && (
                        <AgentThreads
                          jobCount={detail.jobs.length}
                          merged={!!detail.result}
                        />
                      )}

                      {/* Agent summaries */}
                      <div className="flex flex-wrap gap-3">
                        {detail.jobs.map((job) => (
                          <div key={job.id} className="flex-1 min-w-[140px]">
                            <AgentResult job={job} />
                          </div>
                        ))}
                      </div>

                      {/* Merged output */}
                      {detail.result && (
                        <>
                          <Separator className="my-2" />
                          <MergedResults output={detail.result.merged_output} />
                        </>
                      )}
                    </>
                  ) : (
                    <p className="text-sm text-stone">Loading details...</p>
                  )}
                </div>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}

export default function TopicDetailPage() {
  const params = useParams();
  const id = params.id as string;
  const [topic, setTopic] = useState<Topic | null>(null);
  const [runs, setRuns] = useState<Run[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [runningNow, setRunningNow] = useState(false);

  const refreshRuns = useCallback(async () => {
    try {
      const r = await fetchTopicRuns(id);
      setRuns(r);
    } catch {
      // silent
    }
  }, [id]);

  useEffect(() => {
    Promise.all([fetchTopic(id), fetchTopicRuns(id)])
      .then(([t, r]) => {
        setTopic(t);
        setRuns(r);
        setLoading(false);
      })
      .catch((err) => {
        setError(err.message);
        setLoading(false);
      });
  }, [id]);

  const handleRunNow = async () => {
    setRunningNow(true);
    try {
      await runTopicNow(id);
      await refreshRuns();
    } catch {
      // errors are silent here, user can retry
    } finally {
      setRunningNow(false);
    }
  };

  const handleDelete = async () => {
    if (!confirm("Delete this topic? Any running agents will be stopped.")) return;
    try {
      await deleteTopic(id);
      window.location.href = "/";
    } catch {
      // silent
    }
  };

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center min-h-screen">
        <p className="text-stone text-lg">Loading topic...</p>
      </div>
    );
  }

  if (error || !topic) {
    return (
      <div className="flex flex-1 items-center justify-center min-h-screen">
        <div className="text-center">
          <p className="text-berry font-medium text-lg">Could not load topic</p>
          <p className="text-stone mt-1">{error || "Topic not found"}</p>
          <Link href="/" className="text-fern mt-4 inline-block hover:underline">
            Back to topics
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen relative overflow-hidden">
      {/* Atmospheric gradient orbs */}
      <div className="orb orb-lavender w-[400px] h-[400px] -top-32 -right-32" />
      <div className="orb orb-sky w-[350px] h-[350px] top-96 -left-28" />

      <header className="relative z-10 border-b border-border bg-card/80 backdrop-blur-sm">
        <div className="max-w-3xl mx-auto px-6 py-8">
          <Link href="/" className="text-stone text-sm hover:text-fern transition-colors">
            &larr; All topics
          </Link>
          <div className="flex items-start justify-between gap-4 mt-3">
            <div>
              <h1 className="font-heading text-3xl md:text-4xl font-semibold text-ink tracking-tight">
                {topic.name}
              </h1>
              <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5 mt-3 text-xs text-stone">
                <span className="flex items-center gap-1.5">
                  <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M12 6v6h4.5m4.5 0a9 9 0 11-18 0 9 9 0 0118 0z" />
                  </svg>
                  {cronToFriendly(topic.schedule_cron)}
                </span>
                <span className="flex items-center gap-1.5">
                  <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M13.19 8.688a4.5 4.5 0 011.242 7.244l-4.5 4.5a4.5 4.5 0 01-6.364-6.364l1.757-1.757m9.86-2.648a4.5 4.5 0 00-1.242-7.244l-4.5-4.5a4.5 4.5 0 00-6.364 6.364L4.34 8.374" />
                  </svg>
                  {topic.sources.length === 0
                    ? "No sources"
                    : `${topic.sources.length} source${topic.sources.length === 1 ? "" : "s"}`}
                </span>
                <span className="flex items-center gap-1.5">
                  <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M9.75 3.104v5.714a2.25 2.25 0 01-.659 1.591L5 14.5M9.75 3.104c-.251.023-.501.05-.75.082m.75-.082a24.301 24.301 0 014.5 0m0 0v5.714c0 .597.237 1.17.659 1.591L19.8 15.3M14.25 3.104c.251.023.501.05.75.082M19.8 15.3l-1.57.393A9.065 9.065 0 0112 15a9.065 9.065 0 00-6.23.693L5 14.5m14.8.8l1.402 1.402c1.232 1.232.65 3.318-1.067 3.611A48.309 48.309 0 0112 21c-2.773 0-5.491-.235-8.135-.687-1.718-.293-2.3-2.379-1.067-3.61L5 14.5" />
                  </svg>
                  {topic.agent_count} agent{topic.agent_count === 1 ? "" : "s"}
                </span>
              </div>
            </div>
            <div className="flex gap-2 flex-shrink-0">
              <Button
                onClick={handleRunNow}
                disabled={runningNow}
                variant="outline"
                className="text-ink border-border hover:bg-secondary"
              >
                {runningNow ? "Starting..." : "Run now"}
              </Button>
              <Button
                onClick={handleDelete}
                variant="outline"
                className="text-berry border-border hover:bg-berry/10"
              >
                Delete
              </Button>
            </div>
          </div>
        </div>
      </header>

      <main className="relative z-10 max-w-3xl mx-auto px-6 py-10">
        {runs.length === 0 ? (
          <div className="text-center py-16">
            <div className="inline-flex items-center justify-center w-14 h-14 rounded-full bg-ochre/10 mb-5">
              <svg className="w-7 h-7 text-ochre" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 13.5l10.5-11.25L12 10.5h8.25L9.75 21.75 12 13.5H3.75z" />
              </svg>
            </div>
            <p className="text-xl text-ink font-heading">No runs yet</p>
            <p className="text-stone mt-2 max-w-md mx-auto">
              Hit &quot;Run now&quot; above to kick things off, or wait for the next scheduled run.
            </p>
          </div>
        ) : (
          <>
            <h2 className="font-heading text-xl text-ink mb-6">Run History</h2>
            <RunTimeline runs={runs} onRefresh={refreshRuns} />
          </>
        )}

        {topic.sources.length > 0 && (
          <div className="mt-12">
            <h2 className="font-heading text-xl text-ink mb-4">Sources</h2>
            <ul className="space-y-2">
              {topic.sources.map((src, i) => (
                <li key={i} className="flex items-center gap-2 text-sm text-stone">
                  <span className="w-1.5 h-1.5 rounded-full bg-ochre flex-shrink-0" />
                  <span className="truncate">{src.value}</span>
                </li>
              ))}
            </ul>
          </div>
        )}
      </main>
    </div>
  );
}