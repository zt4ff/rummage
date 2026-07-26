export interface Source {
  type: string;
  value: string;
}

export interface NotificationSetting {
  type: "email" | "telegram" | "webhook";
  enabled: boolean;
  target: string;
}

export interface ConditionalRule {
  type: "contains" | "not_contains" | "greater_than" | "less_than" | "equals";
  field: string;
  value: string;
}

export interface Topic {
  id: string;
  name: string;
  schedule_cron: string;
  sources: Source[];
  agent_count: number;
  next_run_at: string;
  created_at: string;
  notifications?: NotificationSetting[];
  condition?: ConditionalRule | null;
}

export interface Run {
  id: string;
  topic_id: string;
  scheduled_for: string;
  status: "pending" | "running" | "completed" | "failed";
  created_at: string;
}

export interface AgentJob {
  id: string;
  run_id: string;
  role: "general_search" | "source_specific";
  status: "pending" | "running" | "completed" | "failed";
  result?: Record<string, unknown>;
  created_at: string;
}

export interface RunResult {
  id: string;
  run_id: string;
  merged_output: unknown;
  created_at: string;
}

export interface RunDetail {
  run: Run;
  jobs: AgentJob[];
  result: RunResult | null;
}

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...init?.headers,
    },
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`API error ${res.status}: ${text}`);
  }
  return res.json();
}

export const fetchTopics = () => api<Topic[]>("/api/topics");
export const fetchTopic = (id: string) => api<Topic>(`/api/topics/${id}`);
export const fetchTopicRuns = (id: string) => api<Run[]>(`/api/topics/${id}/runs`);
export const fetchRunDetail = (id: string) => api<RunDetail>(`/api/runs/${id}`);

export const createTopic = (data: {
  name: string;
  schedule: string;
  sources: Source[];
  agent_count: number;
  notifications?: NotificationSetting[];
  condition?: ConditionalRule | null;
}) =>
  api<Topic>("/api/topics", {
    method: "POST",
    body: JSON.stringify(data),
  });

export const runTopicNow = (id: string) =>
  api<{ run_id: string; status: string }>(`/api/topics/${id}/run-now`, {
    method: "POST",
  });

export const deleteTopic = (id: string) =>
  fetch(`/api/topics/${id}`, { method: "DELETE" }).then((res) => {
    if (!res.ok && res.status !== 204) throw new Error(`Failed to delete topic`);
  });