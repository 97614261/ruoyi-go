export type NodeType = 'textInput' | 'imageInput' | 'textGenerate' | 'imageGenerate' | 'imageProcess' | 'productPreview' | 'export'
export type ValueType = 'text' | 'images' | 'files'
export interface Port { key: string; valueType: ValueType; required: boolean }
export interface NodeDefinition {
  type: NodeType; version: number; label: string; description: string;
  inputs: Port[]; outputs: Port[]; executable: boolean;
}
export interface Graph {
  schemaVersion: 1;
  nodes: { id: string; type: NodeType; version: number; position: { x: number; y: number }; config: Record<string, unknown> }[];
  edges: { id: string; source: string; sourcePort: string; target: string; targetPort: string }[];
}
export interface Bootstrap {
  schemaVersion: number;
  userId: string; userName: string; nickName: string;
  workspace: { workspaceId: string; name: string };
  nodeTypes: NodeDefinition[];
  capabilities: { projectEditing: boolean; execution: boolean; assets: boolean };
  limits: GraphLimits;
  assetLimits: AssetLimits;
  execution?: ExecutionLimits;
}
export interface AssetLimits { maxUploadBytes: number; maxPixels: number; maxDimension: number; maxImagesPerNode: number }
export interface Asset {
  assetId: string; workspaceId: string; parentAssetId: string; source: string; originalName: string; mediaType: string;
  byteSize: number; width: number; height: number; revision: string; deleted: boolean; referenceCount: number; createTime: string;
}
export interface GraphLimits { maxNodes: number; maxEdges: number; maxGraphBytes: number }
export interface ProjectSummary {
  projectId: string; workspaceId: string; name: string; revision: string; deleted: boolean;
  createTime: string; updateTime: string;
}
export interface Project extends ProjectSummary { graph: Graph }
export interface PersonalTemplate {
  templateId: string; name: string; description: string; currentVersion: string; revision: string; deleted: boolean;
  createTime: string; updateTime: string; snapshot?: TemplateVersion;
}
export interface TemplateSlot { nodeId: string; type: 'textInput' | 'imageInput'; label: string; text?: string; assetIds?: string[] }
export interface BatchInput { name: string; bindings: { nodeId: string; text?: string; assetIds?: string[] }[] }
export interface BatchRequest { requestId: string; templateId: string; version: string; name: string; executionMode: string; items: BatchInput[]; quoteId?: string }
export interface BatchImpact { items: number; textCalls: number; imageCalls: number; images: number; reservationMicros: string; testCredits: number }
export interface Batch { batchId: string; templateId: string; templateVersion: string; name: string; executionMode: string; state: string; impact: BatchImpact; createTime: string; billing?: ManagedBilling; items?: { itemNo: number; name: string; state: string; projectId: string; runId: string; errorCode: string }[] }
export interface TemplateVersion {
  templateId: string; version: string; createTime: string; graph?: Graph; slots?: TemplateSlot[];
  outputs?: { nodeId: string; type: NodeType; label: string }[];
}
export interface DraftDocument { name: string; graph: Graph }
export interface Captcha { code: number; captchaEnabled: boolean; uuid?: string; img?: string }
export type RunState = 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelRequested' | 'cancelled' | 'awaitingReconciliation'
export interface ExecutionLimits { mode: string; globalConcurrency: number; userConcurrency: number; providerConcurrency: number; maxPending: number; maxUserPending: number; initialCredits: string; textModel?: string; imageModel?: string; imageAIEnabled?: boolean }
export interface ProviderUsage { model: string; requestId: string; inputTokens: number; outputTokens: number; imageCount: number; estimatedCostMicros: string }
export interface ImageInfo { assetId: string; width: number; height: number; kind: string; format: string; effectiveDpi?: number; warning?: string }
export interface RunNode { nodeId: string; nodeType: NodeType; state: string; attempt: number; output: { testOnly: boolean; text?: string; assetIds?: string[]; usage?: ProviderUsage; images?: ImageInfo[] }; errorCode: string; errorMessage: string }
export interface Run {
  runId: string; projectId: string; projectRevision: string; projectName: string; mode: 'workflow' | 'node' | 'polish' | 'branch'; nodeId: string;
  state: RunState; attempt: number; testOnly: boolean; testScenario: string; reserved: string; spent: string;
  createTime: string; updateTime: string; graph?: Graph; nodes?: RunNode[];
  billing?: ManagedBilling; executionMode?: string;
}
export interface CreditAccount { available: string; held: string; spent: string }
export interface CreditEvent extends CreditAccount { eventId: string; runId: string; kind: string; amount: string; createTime: string }

export interface CreatorEligibility {
  creator: { userId: string; status: 'active' | 'suspended'; expired: boolean; validUntil: string | null; maxConcurrentNodes: number; maxPendingRuns: number; maxDailySubmissions: number; maxStorageBytes: string };
  modelIds: string[]; storage: { usedBytes: string; heldBytes: string }; dailySubmissions: number; businessDate: string;
}
export interface FormalPoints {
  account: CreditAccount; billingEnabled: boolean; total: number;
  rows: { eventId: string; kind: string; amount: string; availableAfter: string; reason: string; createTime: string }[];
}

export interface UsageStats { chargedMicros: string; reservedMicros: string; providerCalls: number; pendingCalls: number; generatedImages: number }
export interface UsageRun extends UsageStats { runId: string; projectId: string; projectName: string; state: RunState; attempt: number; testOnly: boolean; createTime: string }
export interface CreatorUsage {
  summary: UsageStats & { runCount: number; failedRuns: number; pendingRuns: number };
  rows: UsageRun[]; total: number; budgetScope: 'instanceCumulative'; budgetLimitMicros: string;
}

export interface ManagedBilling { scheme: 'managedPointsV1'; held: string; spent: string; released: string }
export interface CreatorModel {
  modelId: string; modelVersionId: string; name: string; kind: 'text' | 'image'; officialModelId: string; default: boolean;
  capabilities: { maxInputBytes?: number; maxOutputTokens?: number; maxPromptChars?: number; maxImages?: number; maxReferenceImages?: number; ratios?: string[] };
  pointRule: { inputPerMillion: string; outputPerMillion: string; perImage: string };
}
export interface ExecutionQuote {
  quoteId: string; kind: 'run' | 'batch' | 'retry'; state: string; expiresAt: string; points: string;
  lines: { itemNo: number; nodeId: string; modelId: string; modelVersionId: string; name: string; points: string; kind?: 'text' | 'image'; officialModelId?: string; images?: number }[];
}
export type QuoteSource = { kind: 'run'; run: Record<string, unknown> } | { kind: 'retry'; runId: string } | { kind: 'batch'; batch: BatchRequest }
