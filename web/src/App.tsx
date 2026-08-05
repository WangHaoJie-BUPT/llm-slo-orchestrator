import {
  Activity,
  AlertTriangle,
  ArrowRight,
  Bot,
  Braces,
  Check,
  ChevronRight,
  CircleStop,
  Clock3,
  ExternalLink,
  FileCheck2,
  Gauge,
  KeyRound,
  LoaderCircle,
  LockKeyhole,
  Network,
  Play,
  RefreshCw,
  Route,
  Send,
  Server,
  ShieldCheck,
  TerminalSquare,
  X,
  Zap,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  applyStrategy,
  cancelJob,
  fetchEvidence,
  fetchEvidenceFile,
  fetchScenarios,
  fetchStatus,
  startScenario,
  streamChat,
  streamJob,
} from "./api";
import type { ChatResult, ConsoleStatus, EvidenceItem, Job, JobEvent, Scenario } from "./types";

type Page = "overview" | "playground" | "scenarios" | "evidence";

const navItems: Array<{ id: Page; label: string; icon: typeof Gauge }> = [
  { id: "overview", label: "Overview", icon: Gauge },
  { id: "playground", label: "Playground", icon: Bot },
  { id: "scenarios", label: "Scenarios", icon: TerminalSquare },
  { id: "evidence", label: "Evidence", icon: FileCheck2 },
];

const strategies = ["round-robin", "random", "least-outstanding", "least-queue", "slo-prefix-affinity"];

function App() {
  const [page, setPage] = useState<Page>("overview");
  const [status, setStatus] = useState<ConsoleStatus | null>(null);
  const [statusError, setStatusError] = useState("");
  const [refreshing, setRefreshing] = useState(false);
  const [token, setTokenState] = useState(() => sessionStorage.getItem("llm-slo-control-token") ?? "");

  const setToken = (value: string) => {
    setTokenState(value);
    if (value) sessionStorage.setItem("llm-slo-control-token", value);
    else sessionStorage.removeItem("llm-slo-control-token");
  };

  const refresh = useCallback(async (quiet = false) => {
    if (!quiet) setRefreshing(true);
    try {
      setStatus(await fetchStatus());
      setStatusError("");
    } catch (error) {
      setStatusError(error instanceof Error ? error.message : "Status request failed");
    } finally {
      if (!quiet) setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
    const timer = window.setInterval(() => void refresh(true), 5000);
    return () => window.clearInterval(timer);
  }, [refresh]);

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark"><Route size={21} /></div>
          <div><strong>LLM SLO</strong><span>Operator Console</span></div>
        </div>
        <nav className="primary-nav" aria-label="Console navigation">
          {navItems.map((item) => {
            const Icon = item.icon;
            return (
              <button key={item.id} className={page === item.id ? "nav-item active" : "nav-item"} onClick={() => setPage(item.id)}>
                <Icon size={18} /><span>{item.label}</span>
              </button>
            );
          })}
        </nav>
        <div className="sidebar-state">
          <div className="sidebar-state-row">
            <span className={`status-dot ${status?.status === "ok" ? "healthy" : "danger"}`} />
            <span>{status?.status === "ok" ? "System connected" : "Connection degraded"}</span>
          </div>
          <div className="sidebar-state-row muted"><ShieldCheck size={15} /><span>{status?.console.demoMode ? "Demo controls on" : "Read-only mode"}</span></div>
        </div>
      </aside>

      <main className="workspace">
        <header className="topbar">
          <div>
            <p className="eyebrow">{status?.router.model || "Router unavailable"}</p>
            <h1>{navItems.find((item) => item.id === page)?.label}</h1>
          </div>
          <div className="topbar-actions">
            <div className="environment-state"><span className={`status-dot ${status?.status === "ok" ? "healthy" : "danger"}`} />{status?.status === "ok" ? "Live" : "Degraded"}</div>
            <button className="icon-button" onClick={() => void refresh()} disabled={refreshing} title="Refresh status" aria-label="Refresh status">
              <RefreshCw size={18} className={refreshing ? "spin" : ""} />
            </button>
          </div>
        </header>

        {statusError && <div className="alert error"><AlertTriangle size={18} /><span>{statusError}</span></div>}
        {page === "overview" && <Overview status={status} token={token} onStatusChange={() => void refresh()} />}
        {page === "playground" && <Playground status={status} />}
        {page === "scenarios" && <Scenarios status={status} token={token} setToken={setToken} />}
        {page === "evidence" && <Evidence />}
      </main>

      <nav className="mobile-nav" aria-label="Mobile console navigation">
        {navItems.map((item) => {
          const Icon = item.icon;
          return <button key={item.id} className={page === item.id ? "active" : ""} onClick={() => setPage(item.id)}><Icon size={19} /><span>{item.label}</span></button>;
        })}
      </nav>
    </div>
  );
}

function Overview({ status, token, onStatusChange }: { status: ConsoleStatus | null; token: string; onStatusChange: () => void }) {
  const [strategy, setStrategy] = useState("");
  const [controlState, setControlState] = useState<"idle" | "saving" | "saved" | "error">("idle");
  const [controlMessage, setControlMessage] = useState("");
  useEffect(() => setStrategy(status?.router.strategy ?? ""), [status?.router.strategy]);
  const endpoints = status?.router.endpoints ?? [];
  const eligible = endpoints.filter((endpoint) => endpoint.eligibleForPolicy).length;
  const running = endpoints.reduce((sum, endpoint) => sum + endpoint.running, 0);
  const waiting = endpoints.reduce((sum, endpoint) => sum + endpoint.waiting, 0);
  const freshness = endpoints.length === 0 ? 0 : Math.round((endpoints.filter((endpoint) => endpoint.metricsFresh).length / endpoints.length) * 100);

  const saveStrategy = async () => {
    setControlState("saving");
    try {
      await applyStrategy(strategy, token);
      setControlState("saved");
      setControlMessage("Strategy updated");
      onStatusChange();
      window.setTimeout(() => setControlState("idle"), 1800);
    } catch (error) {
      setControlState("error");
      setControlMessage(error instanceof Error ? error.message : "Strategy update failed");
    }
  };

  return (
    <div className="page-content">
      <section className="metric-grid" aria-label="Router summary">
        <Metric label="Eligible backends" value={`${eligible}/${endpoints.length}`} detail="healthy, ready, fresh" icon={Server} tone="green" />
        <Metric label="Active requests" value={String(running)} detail={`${waiting} waiting`} icon={Activity} tone="cyan" />
        <Metric label="Metric freshness" value={`${freshness}%`} detail="within policy window" icon={Clock3} tone="amber" />
        <Metric label="Policy generation" value={String(status?.router.generation ?? "-")} detail={status?.router.strategy ?? "unavailable"} icon={Braces} tone="charcoal" />
      </section>

      <section className="section-block">
        <div className="section-heading"><div><p className="eyebrow">Request path</p><h2>Live routing topology</h2></div><span className="section-meta">{status?.router.status ?? "unreachable"}</span></div>
        <div className="topology">
          <div className="topology-node source"><Send size={20} /><div><strong>Console client</strong><span>OpenAI / SSE</span></div></div>
          <ArrowRight className="topology-arrow" size={24} />
          <div className="topology-node router-node"><Network size={21} /><div><strong>Router</strong><span>{status?.router.strategy ?? "offline"}</span></div></div>
          <ArrowRight className="topology-arrow" size={24} />
          <div className="endpoint-stack">
            {endpoints.length === 0 && <div className="empty-state compact">No endpoints reported</div>}
            {endpoints.map((endpoint) => (
              <div className="endpoint-card" key={endpoint.id}>
                <span className={`status-dot ${endpoint.eligibleForPolicy ? "healthy" : "danger"}`} />
                <div className="endpoint-title"><strong>{endpoint.id}</strong><span>{endpoint.model}</span></div>
                <div className="endpoint-load"><strong>{endpoint.running}</strong><span>running</span></div>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="section-block">
        <div className="section-heading"><div><p className="eyebrow">Backend state</p><h2>Endpoint eligibility</h2></div></div>
        <div className="table-wrap">
          <table>
            <thead><tr><th>Endpoint</th><th>Health</th><th>Metrics</th><th>Running</th><th>Waiting</th><th>Outstanding</th><th>KV cache</th><th>TTFT EWMA</th></tr></thead>
            <tbody>
              {endpoints.map((endpoint) => (
                <tr key={endpoint.id}>
                  <td><strong>{endpoint.id}</strong><span className="cell-secondary">{endpoint.model}</span></td>
                  <td><StatusText ok={endpoint.healthy && endpoint.ready} good="Ready" bad="Unavailable" /></td>
                  <td><StatusText ok={endpoint.metricsFresh} good={`${endpoint.metricAgeMs} ms`} bad="Stale" /></td>
                  <td>{endpoint.running}</td><td>{endpoint.waiting}</td><td>{endpoint.outstanding}</td>
                  <td><UsageBar value={endpoint.kvUsage} /></td><td>{endpoint.ttftEwmaMs ? `${endpoint.ttftEwmaMs} ms` : "-"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <section className="section-block split-section">
        <div>
          <div className="section-heading"><div><p className="eyebrow">Runtime policy</p><h2>Routing strategy</h2></div></div>
          <div className="control-row">
            <select value={strategy} onChange={(event) => setStrategy(event.target.value)} disabled={!status?.console.strategyControl} aria-label="Routing strategy">
              {strategies.map((item) => <option key={item}>{item}</option>)}
            </select>
            <button className="primary-button" onClick={() => void saveStrategy()} disabled={!status?.console.strategyControl || !token || controlState === "saving" || strategy === status?.router.strategy}>
              {controlState === "saving" ? <LoaderCircle className="spin" size={17} /> : <Check size={17} />}Apply
            </button>
          </div>
          {controlState === "error" && <p className="field-error">{controlMessage}</p>}
          {controlState === "saved" && <p className="field-success">{controlMessage}</p>}
          {!status?.console.strategyControl && <div className="inline-lock"><LockKeyhole size={15} />Read-only runtime</div>}
        </div>
        <div className="boundary-note"><ShieldCheck size={22} /><div><strong>Evidence boundary</strong><p>{status?.console.evidenceBoundary ?? "Experiment evidence boundary unavailable."}</p></div></div>
      </section>
    </div>
  );
}

function Playground({ status }: { status: ConsoleStatus | null }) {
  const [prompt, setPrompt] = useState("Explain how SLO-aware routing reacts when one backend becomes unhealthy.");
  const [tenant, setTenant] = useState("demo-tenant");
  const [priority, setPriority] = useState("online-normal");
  const [maxTokens, setMaxTokens] = useState(128);
  const [temperature, setTemperature] = useState(0.2);
  const [output, setOutput] = useState("");
  const [result, setResult] = useState<ChatResult | null>(null);
  const [error, setError] = useState("");
  const [running, setRunning] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  const run = async () => {
    const controller = new AbortController();
    abortRef.current = controller;
    setOutput(""); setResult(null); setError(""); setRunning(true);
    try {
      const next = await streamChat({ model: status?.router.model ?? "", prompt, tenant, priority, maxTokens, temperature }, (text) => setOutput((current) => current + text), controller.signal);
      setResult(next);
    } catch (caught) {
      if (!controller.signal.aborted) setError(caught instanceof Error ? caught.message : "Request failed");
    } finally {
      setRunning(false);
      abortRef.current = null;
    }
  };

  return (
    <div className="page-content playground-layout">
      <section className="request-pane">
        <div className="section-heading"><div><p className="eyebrow">OpenAI-compatible</p><h2>Streaming request</h2></div></div>
        <label className="field"><span>Model</span><input value={status?.router.model ?? ""} readOnly /></label>
        <div className="field-grid">
          <label className="field"><span>Tenant</span><input value={tenant} onChange={(event) => setTenant(event.target.value)} maxLength={128} /></label>
          <label className="field"><span>Priority</span><select value={priority} onChange={(event) => setPriority(event.target.value)}><option value="online-high">online-high</option><option value="online-normal">online-normal</option><option value="batch-low">batch-low</option></select></label>
        </div>
        <label className="field prompt-field"><span>Prompt</span><textarea value={prompt} onChange={(event) => setPrompt(event.target.value)} rows={9} /></label>
        <div className="field-grid">
          <label className="field"><span>Max tokens</span><input type="number" min={1} max={2048} value={maxTokens} onChange={(event) => setMaxTokens(Number(event.target.value))} /></label>
          <label className="field"><span>Temperature</span><input type="number" min={0} max={2} step={0.1} value={temperature} onChange={(event) => setTemperature(Number(event.target.value))} /></label>
        </div>
        <div className="command-row">
          <button className="primary-button" onClick={() => void run()} disabled={running || !prompt.trim() || status?.status !== "ok"}>{running ? <LoaderCircle className="spin" size={17} /> : <Play size={17} />}Run</button>
          <button className="secondary-button" onClick={() => abortRef.current?.abort()} disabled={!running}><CircleStop size={17} />Stop</button>
          <button className="text-button" onClick={() => { setOutput(""); setResult(null); setError(""); }} disabled={running}><X size={16} />Clear</button>
        </div>
      </section>

      <section className="response-pane">
        <div className="response-header"><div><p className="eyebrow">Assistant response</p><h2>{running ? "Streaming" : result ? "Completed" : "Ready"}</h2></div><span className={`status-dot ${error ? "danger" : running ? "warning" : result ? "healthy" : "neutral"}`} /></div>
        <div className="response-output" aria-live="polite">{output || <span className="output-placeholder">Response output</span>}{running && <span className="stream-caret" />}</div>
        {error && <div className="alert error"><AlertTriangle size={17} />{error}</div>}
        <div className="response-metrics">
          <ResultMetric label="Endpoint" value={result?.endpoint ?? "-"} />
          <ResultMetric label="TTFT" value={result?.ttftMs != null ? `${result.ttftMs} ms` : "-"} />
          <ResultMetric label="Duration" value={result ? `${result.durationMs} ms` : "-"} />
          <ResultMetric label="Retries" value={result ? String(result.retryCount) : "-"} />
        </div>
        <dl className="request-details"><div><dt>Request ID</dt><dd>{result?.requestId ?? "-"}</dd></div><div><dt>Decision</dt><dd>{result?.reason ?? "-"}</dd></div><div><dt>Generation</dt><dd>{result?.generation ?? "-"}</dd></div><div><dt>HTTP</dt><dd>{result?.status ?? "-"}</dd></div></dl>
      </section>
    </div>
  );
}

function Scenarios({ status, token, setToken }: { status: ConsoleStatus | null; token: string; setToken: (value: string) => void }) {
  const [scenarios, setScenarios] = useState<Scenario[]>([]);
  const [job, setJob] = useState<Job | null>(null);
  const [error, setError] = useState("");
  const streamAbort = useRef<AbortController | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    void fetchScenarios(controller.signal).then((result) => setScenarios(result.items)).catch((caught) => setError(caught instanceof Error ? caught.message : "Scenario index failed"));
    return () => controller.abort();
  }, []);

  const watchJob = (next: Job) => {
    setJob(next);
    streamAbort.current?.abort();
    const controller = new AbortController();
    streamAbort.current = controller;
    void streamJob(next.id, token, (event: JobEvent) => {
      if (event.type === "log" && event.line !== undefined) setJob((current) => current ? { ...current, logs: [...current.logs, event.line!] } : current);
      if (event.job) setJob(event.job);
    }, controller.signal).catch((caught) => {
      if (!controller.signal.aborted) setError(caught instanceof Error ? caught.message : "Job stream failed");
    });
  };

  const runScenario = async (id: string) => {
    setError("");
    try { watchJob(await startScenario(id, token)); }
    catch (caught) { setError(caught instanceof Error ? caught.message : "Scenario failed to start"); }
  };

  return (
    <div className="page-content scenarios-layout">
      <section className="scenario-list">
        <div className="section-heading"><div><p className="eyebrow">Allowlisted jobs</p><h2>Demonstration scenarios</h2></div><span className="section-meta">{status?.console.scenarioControl ? "enabled" : "disabled"}</span></div>
        <label className="field token-field"><span>Control token</span><div className="input-with-icon"><KeyRound size={17} /><input type="password" value={token} onChange={(event) => setToken(event.target.value)} autoComplete="off" /></div></label>
        <div className="scenario-rows">
          {scenarios.map((scenario) => (
            <div className="scenario-row" key={scenario.id}>
              <div className="scenario-icon">{scenario.id === "verify" ? <ShieldCheck size={20} /> : scenario.id === "fault-smoke" ? <Zap size={20} /> : <Activity size={20} />}</div>
              <div className="scenario-copy"><strong>{scenario.name}</strong><span>{scenario.description}</span><small>{scenario.risk}</small></div>
              <button className="icon-button bordered" title={`Run ${scenario.name}`} aria-label={`Run ${scenario.name}`} disabled={!status?.console.scenarioControl || !token || job?.status === "running"} onClick={() => void runScenario(scenario.id)}><Play size={17} /></button>
            </div>
          ))}
        </div>
        {error && <div className="alert error"><AlertTriangle size={17} />{error}</div>}
      </section>

      <section className="job-pane">
        <div className="job-toolbar"><div><p className="eyebrow">Execution</p><h2>{job ? job.id : "No active job"}</h2></div>{job && (job.status === "queued" || job.status === "running") && <button className="secondary-button" onClick={() => void cancelJob(job.id, token)}><CircleStop size={16} />Cancel</button>}</div>
        <div className="job-status-line"><StatusText ok={job?.status === "passed"} good="Passed" bad={job?.status ?? "Idle"} /><span>{job?.scenarioId ?? "-"}</span></div>
        <pre className="job-log">{job?.logs.join("\n") || "$ waiting for scenario selection"}</pre>
        {job?.error && <div className="alert error"><AlertTriangle size={17} />{job.error}</div>}
      </section>
    </div>
  );
}

function Evidence() {
  const [items, setItems] = useState<EvidenceItem[]>([]);
  const [selected, setSelected] = useState<EvidenceItem | null>(null);
  const [content, setContent] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  useEffect(() => {
    const controller = new AbortController();
    void fetchEvidence(controller.signal).then((result) => { setItems(result); setLoading(false); }).catch((caught) => { setError(caught instanceof Error ? caught.message : "Evidence index failed"); setLoading(false); });
    return () => controller.abort();
  }, []);
  const open = async (item: EvidenceItem) => {
    setSelected(item); setContent(""); setError("");
    try { setContent(await fetchEvidenceFile(item.id)); }
    catch (caught) { setError(caught instanceof Error ? caught.message : "Evidence file failed"); }
  };
  const reports = useMemo(() => items.filter((item) => item.kind === "report"), [items]);
  const evidence = useMemo(() => items.filter((item) => item.kind === "evidence"), [items]);
  return (
    <div className="page-content evidence-layout">
      <section className="evidence-index">
        <div className="section-heading"><div><p className="eyebrow">Repository artifacts</p><h2>Evidence index</h2></div><span className="section-meta">{items.length} files</span></div>
        {loading && <div className="loading-row"><LoaderCircle className="spin" size={18} />Loading</div>}
        <EvidenceGroup label="Generated reports" items={reports} selected={selected} onOpen={open} />
        <EvidenceGroup label="Evidence summaries" items={evidence} selected={selected} onOpen={open} />
      </section>
      <section className="evidence-viewer">
        <div className="viewer-toolbar"><div><p className="eyebrow">Artifact</p><h2>{selected?.title ?? "Select a file"}</h2></div>{selected && <a className="icon-button bordered" href={`/api/v1/evidence/file?id=${encodeURIComponent(selected.id)}`} target="_blank" rel="noreferrer" title="Open artifact" aria-label="Open artifact"><ExternalLink size={17} /></a>}</div>
        {error && <div className="alert error"><AlertTriangle size={17} />{error}</div>}
        <pre className="evidence-content">{content || "No artifact selected"}</pre>
      </section>
    </div>
  );
}

function Metric({ label, value, detail, icon: Icon, tone }: { label: string; value: string; detail: string; icon: typeof Gauge; tone: string }) {
  return <article className={`metric-card ${tone}`}><div className="metric-icon"><Icon size={19} /></div><div><span>{label}</span><strong>{value}</strong><small>{detail}</small></div></article>;
}

function StatusText({ ok, good, bad }: { ok: boolean; good: string; bad: string }) {
  return <span className={`status-text ${ok ? "ok" : "bad"}`}><span className={`status-dot ${ok ? "healthy" : "danger"}`} />{ok ? good : bad}</span>;
}

function UsageBar({ value }: { value: number }) {
  const percentage = Math.max(0, Math.min(100, Math.round(value * 100)));
  return <div className="usage-cell"><div className="usage-track"><span style={{ width: `${percentage}%` }} /></div><small>{percentage}%</small></div>;
}

function ResultMetric({ label, value }: { label: string; value: string }) {
  return <div><span>{label}</span><strong>{value}</strong></div>;
}

function EvidenceGroup({ label, items, selected, onOpen }: { label: string; items: EvidenceItem[]; selected: EvidenceItem | null; onOpen: (item: EvidenceItem) => void }) {
  return <div className="evidence-group"><h3>{label}</h3>{items.map((item) => <button key={item.id} className={selected?.id === item.id ? "evidence-row active" : "evidence-row"} onClick={() => void onOpen(item)}><FileCheck2 size={17} /><span><strong>{item.title}</strong><small>{item.evidenceLevel || item.kind}{item.status ? ` · ${item.status}` : ""}</small></span><ChevronRight size={16} /></button>)}</div>;
}

export default App;
