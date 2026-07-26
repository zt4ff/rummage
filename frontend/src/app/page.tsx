"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { fetchTopics, fetchTopicRuns, type Topic, type Run } from "@/lib/api";
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

export default function TopicsPage() {
  const [topics, setTopics] = useState<Topic[]>([]);
  const [lastRuns, setLastRuns] = useState<Record<string, Run>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetchTopics()
      .then((data) => {
        setTopics(data);
        setLoading(false);
        data.forEach((topic) => {
          fetchTopicRuns(topic.id)
            .then((runs) => {
              if (runs.length > 0) {
                setLastRuns((prev) => ({ ...prev, [topic.id]: runs[0] }));
              }
            })
            .catch(() => {});
        });
      })
      .catch((err) => {
        setError(err.message);
        setLoading(false);
      });
  }, []);

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center min-h-screen">
        <p className="text-stone text-lg font-body">Loading topics...</p>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-1 items-center justify-center min-h-screen">
        <div className="text-center">
          <p className="text-berry font-medium text-lg">Could not load topics</p>
          <p className="text-stone mt-1">{error}</p>
          <p className="text-stone mt-3 text-sm">Make sure the backend is running on port 3001.</p>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen relative overflow-hidden">
      {/* Atmospheric gradient orbs */}
      <div className="orb orb-mint w-[500px] h-[500px] -top-40 -right-40" />
      <div className="orb orb-peach w-[400px] h-[400px] top-60 -left-32" />

      <header className="relative z-10 border-b border-border bg-card/80 backdrop-blur-sm">
        <div className="max-w-3xl mx-auto px-6 py-8">
          <h1 className="font-heading text-3xl md:text-4xl font-semibold text-ink tracking-tight">
            Research Topics
          </h1>
          <p className="mt-2 text-stone text-base">
            Things worth keeping an eye on. Each topic runs its agents on schedule and merges what they find.
          </p>
        </div>
      </header>

      <main className="relative z-10 max-w-3xl mx-auto px-6 py-10">
        {topics.length === 0 ? (
          <div className="text-center py-20">
            <div className="inline-flex items-center justify-center w-16 h-16 rounded-full bg-fern/10 mb-6">
              <svg className="w-8 h-8 text-fern" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 6.042A8.967 8.967 0 006 3.75c-1.052 0-2.062.18-3 .512v14.25A8.987 8.987 0 016 18c2.305 0 4.408.867 6 2.292m0-14.25a8.966 8.966 0 016-2.292c1.052 0 2.062.18 3 .512v14.25A8.987 8.987 0 0018 18a8.967 8.967 0 00-6 2.292m0-14.25v14.25" />
              </svg>
            </div>
            <p className="text-xl text-ink font-heading">No topics yet</p>
            <p className="text-stone mt-2 max-w-md mx-auto">
              Set one up in a minute. Pick a subject, choose a schedule, and let the agents do the digging.
            </p>
            <Link
              href="/topics/new"
              className="inline-block mt-6 px-6 py-3 bg-ink text-white rounded-full font-medium hover:bg-ink/90 transition-colors text-sm"
            >
              Create your first topic
            </Link>
          </div>
        ) : (
          <div className="space-y-3">
            {topics.map((topic) => (
              <Link key={topic.id} href={`/topics/${topic.id}`}>
                <Card className="hover:shadow-md transition-all cursor-pointer group result-card">
                  <CardHeader className="pb-2">
                    <div className="flex items-start justify-between gap-4">
                      <CardTitle className="font-heading text-lg text-ink group-hover:text-fern transition-colors">
                        {topic.name}
                      </CardTitle>
                      {lastRuns[topic.id] && (
                        <StatusBadge status={lastRuns[topic.id].status} />
                      )}
                    </div>
                  </CardHeader>
                  <CardContent>
                    <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-xs text-stone">
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
                  </CardContent>
                </Card>
              </Link>
            ))}
          </div>
        )}

        {topics.length > 0 && (
          <div className="mt-10 text-center">
            <Link
              href="/topics/new"
              className="inline-block px-5 py-2.5 border border-border rounded-full text-ink text-sm font-medium hover:bg-secondary transition-colors"
            >
              Add another topic
            </Link>
          </div>
        )}
      </main>
    </div>
  );
}