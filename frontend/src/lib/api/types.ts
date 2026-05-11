/**
 * TypeScript mirrors of the Go DTOs in internal/server/types.go. Fields use
 * camelCase because that is how the Go JSON tags serialize them.
 *
 * Types are hand-written rather than codegen'd. With ~15 DTOs and a single
 * consumer the maintenance cost of a generator outweighs the payoff;
 * keeping the shapes here also lets the SPA document expected values
 * directly where it uses them.
 */

export interface Session {
  profile: string
  cluster: string
  secret: string
  database: string
  bedrockModel: string
  bedrockLanguage: string
  /**
   * Tri-state production flag: undefined = user has not answered,
   * true / false = explicit choice. When true the SPA paints the
   * ConnectionBar with a warning colour.
   */
  isProduction?: boolean
  /**
   * Tri-state read-only flag. undefined is treated as TRUE server-side
   * (fresh installs are safe by default). When effectively true, the
   * backend rejects any SQL whose leading keyword is not SELECT / WITH /
   * SHOW / EXPLAIN / DESCRIBE / DESC / TABLE / VALUES with an
   * errCodeReadOnly (HTTP 403).
   */
  isReadOnly?: boolean
  /**
   * When true, AI-generated SQL that the server classifies as read-only is
   * dispatched to /api/execute the moment Bedrock returns it — no manual
   * Run press required. undefined / false leaves the existing review-then-
   * run flow intact. Manual editor SQL is unaffected.
   */
  autoRunReadOnly?: boolean
}

export interface Health {
  status: string
}

export interface Profiles {
  profiles: string[]
}

export interface ClusterInfo {
  identifier: string
  arn: string
  engine: string
  endpoint: string
  masterUserSecretArn?: string
}

export interface Clusters {
  clusters: ClusterInfo[]
}

export interface SecretInfo {
  name: string
  arn: string
  description?: string
}

export interface Secrets {
  secrets: SecretInfo[]
  suggested: boolean
}

export interface Databases {
  history: string[]
}

export interface ExecuteRequestBody {
  profile: string
  cluster: string
  secret: string
  database: string
  sql: string
  // See ExecuteResponse.NeedsConfirmation in internal/server/types.go
  // for the confirmation handshake.
  confirmed?: boolean
}

export interface ExecuteResponseBody {
  columns: string[]
  rows: unknown[][]
  updated: number
  durationMs: number
  // See ExecuteResponse.NeedsConfirmation in internal/server/types.go.
  needsConfirmation?: boolean
  confirmReason?: string
}

export interface SchemaColumn {
  schema: string
  table: string
  name: string
  type: string
}

export interface Schema {
  cluster: string
  database: string
  fetchedAt: string
  columns: SchemaColumn[]
  fromCache: boolean
}

export interface SchemaRefreshBody {
  profile: string
  cluster: string
  secret: string
  database: string
}

export interface HistoryEntry {
  profile: string
  database: string
  sql: string
  at: string
  ok: boolean
  durationMs: number
  error?: string
  favorite?: boolean
}

export interface History {
  entries: HistoryEntry[]
}

export interface FavoriteBody {
  at: string
  favorite: boolean
}

export interface ModelInfo {
  id: string
  name: string
  description?: string
}

export interface Models {
  models: ModelInfo[]
}

export type MessageRole = 'user' | 'assistant'

export interface Message {
  role: MessageRole
  text: string
}

interface AIRequestBase {
  profile: string
  cluster: string
  database: string
  modelId: string
  language: string
}

export interface AskRequestBody extends AIRequestBase {
  messages: Message[]
}

export interface AskResponseBody {
  sql: string
  /** Server-side runner.IsReadOnlySQL classification of the returned SQL. */
  isReadOnly: boolean
}

export interface ExplainRequestBody extends AIRequestBase {
  sql: string
  errorMsg: string
}

export interface ReviewRequestBody extends AIRequestBase {
  sql: string
  focus?: string
}

export interface AnalyzeRequestBody extends AIRequestBase {
  sql: string
  resultBlob: string
  focus?: string
}

export interface TextResponse {
  text: string
}

export interface ApiErrorPayload {
  error: {
    code: string
    message: string
  }
}
