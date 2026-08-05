import { completionText, consumeSSEBuffer } from "./sse";
import type { ChatResult, ConsoleStatus, EvidenceItem, Job, JobEvent, Scenario } from "./types";

async function errorMessage(response: Response): Promise<string> {
  try {
    const body = (await response.json()) as { error?: { message?: string } };
    return body.error?.message ?? `HTTP ${response.status}`;
  } catch {
    return `HTTP ${response.status}`;
  }
}

export async function fetchStatus(signal?: AbortSignal): Promise<ConsoleStatus> {
  const response = await fetch("/api/v1/status", { signal, cache: "no-store" });
  if (!response.ok) throw new Error(await errorMessage(response));
  return response.json() as Promise<ConsoleStatus>;
}

export async function fetchEvidence(signal?: AbortSignal): Promise<EvidenceItem[]> {
  const response = await fetch("/api/v1/evidence", { signal, cache: "no-store" });
  if (!response.ok) throw new Error(await errorMessage(response));
  const body = (await response.json()) as { items: EvidenceItem[] };
  return body.items;
}

export async function fetchEvidenceFile(id: string, signal?: AbortSignal): Promise<string> {
  const response = await fetch(`/api/v1/evidence/file?id=${encodeURIComponent(id)}`, { signal, cache: "no-store" });
  if (!response.ok) throw new Error(await errorMessage(response));
  return response.text();
}

export async function fetchScenarios(signal?: AbortSignal): Promise<{ items: Scenario[]; enabled: boolean }> {
  const response = await fetch("/api/v1/scenarios", { signal, cache: "no-store" });
  if (!response.ok) throw new Error(await errorMessage(response));
  return response.json() as Promise<{ items: Scenario[]; enabled: boolean }>;
}

export async function applyStrategy(strategy: string, token: string): Promise<void> {
  const response = await fetch("/api/v1/control/strategy", {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
    body: JSON.stringify({ strategy }),
  });
  if (!response.ok) throw new Error(await errorMessage(response));
}

export async function startScenario(id: string, token: string): Promise<Job> {
  const response = await fetch(`/api/v1/scenarios/${encodeURIComponent(id)}/run`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!response.ok) throw new Error(await errorMessage(response));
  return response.json() as Promise<Job>;
}

export async function cancelJob(id: string, token: string): Promise<void> {
  const response = await fetch(`/api/v1/jobs/${encodeURIComponent(id)}/cancel`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!response.ok) throw new Error(await errorMessage(response));
}

export async function streamJob(
  id: string,
  token: string,
  onEvent: (event: JobEvent) => void,
  signal: AbortSignal,
): Promise<void> {
  const response = await fetch(`/api/v1/jobs/${encodeURIComponent(id)}/events`, {
    headers: { Authorization: `Bearer ${token}` },
    signal,
    cache: "no-store",
  });
  if (!response.ok || !response.body) throw new Error(await errorMessage(response));
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    const { value, done } = await reader.read();
    buffer += decoder.decode(value, { stream: !done });
    const parsed = consumeSSEBuffer(buffer);
    buffer = parsed.rest;
    for (const message of parsed.messages) onEvent(JSON.parse(message.data) as JobEvent);
    if (done) return;
  }
}

type ChatInput = {
  model: string;
  prompt: string;
  tenant: string;
  priority: string;
  maxTokens: number;
  temperature: number;
};

export async function streamChat(
  input: ChatInput,
  onText: (text: string) => void,
  signal: AbortSignal,
): Promise<ChatResult> {
  const started = performance.now();
  let firstTokenAt: number | null = null;
  const response = await fetch("/api/v1/chat/completions", {
    method: "POST",
    signal,
    headers: {
      "Content-Type": "application/json",
      "X-Tenant-ID": input.tenant,
      "X-Request-Priority": input.priority,
    },
    body: JSON.stringify({
      model: input.model,
      stream: true,
      max_tokens: input.maxTokens,
      temperature: input.temperature,
      messages: [{ role: "user", content: input.prompt }],
    }),
  });
  if (!response.ok || !response.body) throw new Error(await errorMessage(response));
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    const { value, done } = await reader.read();
    buffer += decoder.decode(value, { stream: !done });
    const parsed = consumeSSEBuffer(buffer);
    buffer = parsed.rest;
    for (const message of parsed.messages) {
      const text = completionText(message.data);
      if (text) {
        firstTokenAt ??= performance.now();
        onText(text);
      }
    }
    if (done) break;
  }
  const ended = performance.now();
  return {
    endpoint: response.headers.get("X-LLM-Endpoint") ?? "-",
    requestId: response.headers.get("X-LLM-Request-ID") ?? "-",
    reason: response.headers.get("X-LLM-Decision-Reason") ?? "-",
    generation: response.headers.get("X-LLM-Policy-Generation") ?? "-",
    retryCount: Number(response.headers.get("X-LLM-Retry-Count") ?? 0),
    status: response.status,
    ttftMs: firstTokenAt === null ? null : Math.round(firstTokenAt - started),
    durationMs: Math.round(ended - started),
  };
}
