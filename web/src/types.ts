export type Endpoint = {
  id: string;
  model: string;
  healthy: boolean;
  ready: boolean;
  metricsFresh: boolean;
  healthCheckAgeMs: number;
  metricAgeMs: number;
  running: number;
  waiting: number;
  outstanding: number;
  kvUsage: number;
  ttftEwmaMs: number;
  eligibleForPolicy: boolean;
};

export type RouterStatus = {
  status: string;
  model: string;
  strategy: string;
  generation: number;
  collectedAt: string;
  objectives: {
    ttftP95Ms: number;
    tpotP95Ms: number;
    availability: number;
  };
  endpoints: Endpoint[];
};

export type ConsoleStatus = {
  status: "ok" | "degraded";
  checkedAt: string;
  router: RouterStatus;
  routerError?: string;
  evidenceCount: number;
  console: {
    demoMode: boolean;
    controlsEnabled: boolean;
    strategyControl: boolean;
    scenarioControl: boolean;
    dashboardUrl: string;
    evidenceBoundary: string;
  };
};

export type EvidenceItem = {
  id: string;
  title: string;
  kind: "report" | "evidence";
  status?: string;
  evidenceLevel?: string;
  updatedAt: string;
  size: number;
};

export type Scenario = {
  id: string;
  name: string;
  description: string;
  risk: "read-mostly" | "local-processes";
};

export type Job = {
  id: string;
  scenarioId: string;
  status: "queued" | "running" | "passed" | "failed" | "cancelled";
  createdAt: string;
  startedAt?: string;
  endedAt?: string;
  error?: string;
  logs: string[];
};

export type JobEvent = {
  type: "state" | "log";
  line?: string;
  job?: Job;
};

export type ChatResult = {
  endpoint: string;
  requestId: string;
  reason: string;
  generation: string;
  retryCount: number;
  status: number;
  ttftMs: number | null;
  durationMs: number;
};
