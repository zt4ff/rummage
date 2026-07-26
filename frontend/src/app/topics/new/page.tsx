"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { createTopic, runTopicNow, type Source, type NotificationSetting, type ConditionalRule } from "@/lib/api";

const PRESET_SCHEDULES = [
  { label: "Run once, immediately", value: "__immediate__" },
  { label: "Every morning", value: "0 8 * * *" },
  { label: "Every evening", value: "0 18 * * *" },
  { label: "Every Saturday morning", value: "0 9 * * 6" },
  { label: "Every Monday morning", value: "0 9 * * 1" },
  { label: "Weekdays at 9 AM", value: "0 9 * * 1-5" },
  { label: "Twice a week (Mon & Thu)", value: "0 9 * * 1,4" },
  { label: "Every noon", value: "0 12 * * *" },
  { label: "Custom cron", value: "custom" },
];

const NOTIFICATION_CHANNELS = [
  { type: "email" as const, label: "Email", icon: "mail", placeholder: "you@example.com" },
  { type: "telegram" as const, label: "Telegram", icon: "message", placeholder: "@username or chat ID" },
  { type: "webhook" as const, label: "Webhook", icon: "link", placeholder: "https://your-webhook-url.com" },
];

const CONDITION_OPERATORS = [
  { value: "contains", label: "contains" },
  { value: "not_contains", label: "does not contain" },
  { value: "greater_than", label: "is greater than" },
  { value: "less_than", label: "is less than" },
  { value: "equals", label: "equals" },
];

function NotificationIcon({ type }: { type: string }) {
  if (type === "email") {
    return (
      <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M21.75 6.75v10.5a2.25 2.25 0 01-2.25 2.25h-15a2.25 2.25 0 01-2.25-2.25V6.75m19.5 0A2.25 2.25 0 0019.5 4.5h-15a2.25 2.25 0 00-2.25 2.25m19.5 0v.243a2.25 2.25 0 01-1.07 1.916l-7.5 4.615a2.25 2.25 0 01-2.36 0L3.32 8.91a2.25 2.25 0 01-1.07-1.916V6.75" />
      </svg>
    );
  }
  if (type === "telegram") {
    return (
      <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M8.625 12a.375.375 0 11-.75 0 .375.375 0 01.75 0zm0 0H8.25m4.125 0a.375.375 0 11-.75 0 .375.375 0 01.75 0zm0 0H12m4.125 0a.375.375 0 11-.75 0 .375.375 0 01.75 0zm0 0h-.375M21 12c0 4.556-4.03 8.25-9 8.25a9.764 9.764 0 01-2.555-.337A5.972 5.972 0 015.41 20.97a5.969 5.969 0 01-.474-.065 4.48 4.48 0 00.978-2.025c.09-.457-.133-.901-.467-1.226C3.93 16.178 3 14.189 3 12c0-4.556 4.03-8.25 9-8.25s9 3.694 9 8.25z" />
      </svg>
    );
  }
  return (
    <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
      <path strokeLinecap="round" strokeLinejoin="round" d="M13.19 8.688a4.5 4.5 0 011.242 7.244l-4.5 4.5a4.5 4.5 0 01-6.364-6.364l1.757-1.757m9.86-2.648a4.5 4.5 0 00-1.242-7.244l-4.5-4.5a4.5 4.5 0 00-6.364 6.364L4.34 8.374" />
    </svg>
  );
}

export default function NewTopicPage() {
  const router = useRouter();
  const [name, setName] = useState("");
  const [scheduleKey, setScheduleKey] = useState("__immediate__");
  const [customCron, setCustomCron] = useState("");
  const [sources, setSources] = useState<Source[]>([]);
  const [sourceInput, setSourceInput] = useState("");
  const [agentCount, setAgentCount] = useState(3);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Notification settings
  const [notifications, setNotifications] = useState<NotificationSetting[]>([
    { type: "email", enabled: false, target: "" },
    { type: "telegram", enabled: false, target: "" },
    { type: "webhook", enabled: false, target: "" },
  ]);

  // Conditional rule
  const [hasCondition, setHasCondition] = useState(false);
  const [condition, setCondition] = useState<ConditionalRule>({
    type: "contains",
    field: "title",
    value: "",
  });

  const isImmediate = scheduleKey === "__immediate__";
  const effectiveCron = isImmediate ? "0 9 * * 6" : scheduleKey === "custom" ? customCron : scheduleKey;

  const addSource = () => {
    const val = sourceInput.trim();
    if (!val) return;
    setSources((prev) => [...prev, { type: "url", value: val }]);
    setSourceInput("");
  };

  const removeSource = (idx: number) => {
    setSources((prev) => prev.filter((_, i) => i !== idx));
  };

  const toggleNotification = (type: NotificationSetting["type"]) => {
    setNotifications((prev) =>
      prev.map((n) =>
        n.type === type ? { ...n, enabled: !n.enabled } : n
      )
    );
  };

  const updateNotificationTarget = (type: NotificationSetting["type"], target: string) => {
    setNotifications((prev) =>
      prev.map((n) =>
        n.type === type ? { ...n, target } : n
      )
    );
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) {
      setError("Give your topic a name so you can recognize it later.");
      return;
    }
    if (!isImmediate && !effectiveCron.trim()) {
      setError("Pick a schedule, or write a custom cron expression.");
      return;
    }

    setSaving(true);
    setError(null);
    try {
      const activeNotifications = notifications.filter((n) => n.enabled && n.target.trim());

      const topic = await createTopic({
        name: name.trim(),
        schedule: effectiveCron.trim(),
        sources,
        agent_count: agentCount,
        notifications: activeNotifications.length > 0 ? activeNotifications : undefined,
        condition: hasCondition && condition.value.trim() ? condition : null,
      });

      if (isImmediate) {
        await runTopicNow(topic.id);
      }

      router.push(`/topics/${topic.id}`);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Something went wrong. Try again.");
      setSaving(false);
    }
  };

  return (
    <div className="min-h-screen relative overflow-hidden">
      {/* Atmospheric gradient orbs */}
      <div className="orb orb-mint w-[450px] h-[450px] -top-36 -left-36" />
      <div className="orb orb-rose w-[350px] h-[350px] top-80 -right-28" />

      <header className="relative z-10 border-b border-border bg-card/80 backdrop-blur-sm">
        <div className="max-w-2xl mx-auto px-6 py-8">
          <Link href="/" className="text-stone text-sm hover:text-fern transition-colors">
            &larr; All topics
          </Link>
          <h1 className="font-heading text-3xl md:text-4xl font-semibold text-ink tracking-tight mt-3">
            New topic
          </h1>
          <p className="mt-2 text-stone text-base">
            Pick something you want to keep tabs on. The agents will research it on schedule and bring back what they find.
          </p>
        </div>
      </header>

      <main className="relative z-10 max-w-2xl mx-auto px-6 py-10">
        <form onSubmit={handleSubmit} className="space-y-8">
          {/* Name */}
          <div className="space-y-2">
            <Label htmlFor="name" className="text-sm font-medium text-ink">
              What are you researching?
            </Label>
            <Input
              id="name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. Developments in fusion energy"
              className="bg-card text-ink"
            />
          </div>

          {/* Schedule */}
          <div className="space-y-2">
            <Label className="text-sm font-medium text-ink">
              How often should it run?
            </Label>
            <Select value={scheduleKey} onValueChange={(v) => v && setScheduleKey(v)}>
              <SelectTrigger className="bg-card text-ink">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {PRESET_SCHEDULES.map((s) => (
                  <SelectItem key={s.value} value={s.value}>
                    {s.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {scheduleKey === "custom" && (
              <Input
                value={customCron}
                onChange={(e) => setCustomCron(e.target.value)}
                placeholder="e.g. 0 9 * * 1-5"
                className="bg-card text-ink font-mono text-sm mt-2"
              />
            )}
            {isImmediate && (
              <p className="text-xs text-fern">
                The agents will start digging right now. You can set a recurring schedule later.
              </p>
            )}
          </div>

          {/* Sources */}
          <div className="space-y-2">
            <Label className="text-sm font-medium text-ink">
              Sources to check (optional)
            </Label>
            <p className="text-xs text-stone">
              Add URLs the source-specific agent should fetch. Leave empty and the agents will search freely.
            </p>
            <div className="flex gap-2">
              <Input
                value={sourceInput}
                onChange={(e) => setSourceInput(e.target.value)}
                placeholder="https://example.com/article"
                className="bg-card text-ink flex-1"
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    addSource();
                  }
                }}
              />
              <Button type="button" variant="outline" onClick={addSource} className="text-ink">
                Add
              </Button>
            </div>
            {sources.length > 0 && (
              <ul className="space-y-1 mt-2">
                {sources.map((src, i) => (
                  <li key={i} className="flex items-center justify-between text-sm text-stone bg-secondary rounded-md px-3 py-1.5">
                    <span className="truncate">{src.value}</span>
                    <button
                      type="button"
                      onClick={() => removeSource(i)}
                      className="text-berry hover:text-berry/80 ml-2 flex-shrink-0 text-xs"
                    >
                      remove
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>

          {/* Agent count */}
          <div className="space-y-2">
            <Label htmlFor="agent-count" className="text-sm font-medium text-ink">
              How many agents?
            </Label>
            <p className="text-xs text-stone">
              More agents means broader coverage. They each search differently, then merge their findings.
            </p>
            <Select value={String(agentCount)} onValueChange={(v) => v && setAgentCount(parseInt(v, 10))}>
              <SelectTrigger className="bg-card text-ink w-32">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {[2, 3, 4, 5].map((n) => (
                  <SelectItem key={n} value={String(n)}>
                    {n} agents
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <Separator />

          {/* Notifications */}
          <div className="space-y-4">
            <div>
              <Label className="text-sm font-medium text-ink">
                Notify me when results arrive
              </Label>
              <p className="text-xs text-stone mt-1">
                Choose where to send updates. You can enable multiple channels.
              </p>
            </div>

            <div className="space-y-3">
              {NOTIFICATION_CHANNELS.map((channel) => {
                const setting = notifications.find((n) => n.type === channel.type)!;
                return (
                  <div
                    key={channel.type}
                    className={`rounded-lg border p-4 transition-colors ${
                      setting.enabled
                        ? "border-fern/30 bg-fern/5"
                        : "border-border bg-card"
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-3">
                        <div className={`w-8 h-8 rounded-full flex items-center justify-center ${
                          setting.enabled ? "bg-fern/10 text-fern" : "bg-secondary text-stone"
                        }`}>
                          <NotificationIcon type={channel.type} />
                        </div>
                        <div>
                          <p className="text-sm font-medium text-ink">{channel.label}</p>
                          <p className="text-xs text-stone">
                            {channel.type === "email" && "Get results delivered to your inbox"}
                            {channel.type === "telegram" && "Receive updates via Telegram"}
                            {channel.type === "webhook" && "Send results to a URL"}
                          </p>
                        </div>
                      </div>
                      <button
                        type="button"
                        onClick={() => toggleNotification(channel.type)}
                        className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                          setting.enabled ? "bg-fern" : "bg-stone/30"
                        }`}
                      >
                        <span
                          className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                            setting.enabled ? "translate-x-6" : "translate-x-1"
                          }`}
                        />
                      </button>
                    </div>

                    {setting.enabled && (
                      <div className="mt-3">
                        <Input
                          value={setting.target}
                          onChange={(e) => updateNotificationTarget(channel.type, e.target.value)}
                          placeholder={channel.placeholder}
                          className="bg-card text-ink text-sm"
                        />
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>

          <Separator />

          {/* Conditional trigger */}
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <Label className="text-sm font-medium text-ink">
                  Only notify if a condition is met
                </Label>
                <p className="text-xs text-stone mt-1">
                  Set a rule to filter what triggers a notification. Useful for monitoring specific changes.
                </p>
              </div>
              <button
                type="button"
                onClick={() => setHasCondition(!hasCondition)}
                className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
                  hasCondition ? "bg-fern" : "bg-stone/30"
                }`}
              >
                <span
                  className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                    hasCondition ? "translate-x-6" : "translate-x-1"
                  }`}
                />
              </button>
            </div>

            {hasCondition && (
              <div className="rounded-lg border border-fern/20 bg-fern/5 p-4 space-y-3">
                <div className="grid grid-cols-3 gap-2">
                  <div className="space-y-1">
                    <Label className="text-xs text-stone">Look in</Label>
                    <Select
                      value={condition.field}
                      onValueChange={(v) => v && setCondition((prev) => ({ ...prev, field: v }))}
                    >
                      <SelectTrigger className="bg-card text-ink text-sm h-9">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="title">Title</SelectItem>
                        <SelectItem value="summary">Summary</SelectItem>
                        <SelectItem value="venue">Venue</SelectItem>
                        <SelectItem value="date">Date</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>

                  <div className="space-y-1">
                    <Label className="text-xs text-stone">Condition</Label>
                    <Select
                      value={condition.type}
                      onValueChange={(v) => v && setCondition((prev) => ({ ...prev, type: v as ConditionalRule["type"] }))}
                    >
                      <SelectTrigger className="bg-card text-ink text-sm h-9">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {CONDITION_OPERATORS.map((op) => (
                          <SelectItem key={op.value} value={op.value}>
                            {op.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>

                  <div className="space-y-1">
                    <Label className="text-xs text-stone">Value</Label>
                    <Input
                      value={condition.value}
                      onChange={(e) => setCondition((prev) => ({ ...prev, value: e.target.value }))}
                      placeholder="e.g. React 19"
                      className="bg-card text-ink text-sm h-9"
                    />
                  </div>
                </div>

                <p className="text-xs text-stone">
                  Example: &quot;If the title contains React 19, notify me. Otherwise, no worries.&quot;
                </p>
              </div>
            )}
          </div>

          {/* Error */}
          {error && (
            <p className="text-berry text-sm">{error}</p>
          )}

          {/* Submit */}
          <Button
            type="submit"
            disabled={saving}
            className="bg-ink hover:bg-ink/90 text-white px-8 py-6 text-base font-medium rounded-full"
          >
            {saving
              ? isImmediate
                ? "Creating and starting..."
                : "Creating..."
              : isImmediate
                ? "Create and run now"
                : "Start tracking this topic"}
          </Button>
        </form>
      </main>
    </div>
  );
}