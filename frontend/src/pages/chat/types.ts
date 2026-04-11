export type ChatSession = {
  id: string;
  userId: string;
  title: string;
  status: string;
  summary: string;
  maxHistoryTurns: number;
  preferSummary: boolean;
  allowToolContext: boolean;
  maxInputTokens: number;
  maxOutputTokens: number;
  lastDialogId: string;
  dialogCount: number;
  turnCount: number;
  lastDialogAt: number;
  createdAt: number;
  updatedAt: number;
};

export type ChatPart = {
  type: string;
  text?: string;
  blockId?: string;
  language?: string;
  toolCallId?: string;
  toolName?: string;
  format?: string;
  status?: string;
  arguments?: Record<string, any>;
  result?: any;
  actionId?: string;
  component?: string;
  props?: Record<string, any>;
  code?: string;
  message?: string;
  collapsed?: boolean;
  append?: boolean;
  hasResult?: boolean;
};

export type ChatContextMeta = {
  strategy: string;
  summaryUsed: boolean;
  includedDialogIds: string[];
  toolContextIncluded: boolean;
  truncated: boolean;
  inputTokens: number;
  reservedOutputTokens: number;
};

export type ChatDialog = {
  id: string;
  sessionId: string;
  dialogId: string;
  role: 'question' | 'answer';
  status: string;
  contentText: string;
  parts: ChatPart[];
  contextMeta?: ChatContextMeta;
  seq: number;
  provider?: string;
  model?: string;
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
  canRegenerate: boolean;
  errorCode?: string;
  errorMessage?: string;
  startedAt: number;
  completedAt: number;
  createdAt: number;
  updatedAt: number;
};

export type ChatSessionDetail = {
  session: ChatSession;
  dialogs: ChatDialog[];
};

export type CreateChatDialogResponse = {
  question: ChatDialog;
  answer: ChatDialog;
};

export type ChatDeltaEvent = {
  v: number;
  id: string;
  sessionId: string;
  dialogId: string;
  seq: number;
  type: string;
  phase: string;
  ts: number;
  delta: Record<string, any>;
  meta?: Record<string, any>;
};

export type GeneratedStrategyParam = {
  name: string;
  description?: string;
  type: string;
  required?: boolean;
  default?: any;
};

export type GeneratedStrategySignal = {
  id: string;
  type: string;
  scope?: string;
  exchange?: string;
  symbol?: string;
  props?: Record<string, any>;
};

export type GeneratedStrategyPayload = {
  name: string;
  code: string;
  description: string;
  params: GeneratedStrategyParam[];
  signals: GeneratedStrategySignal[];
};

export type ChatModelOption = {
  id: string;
  ownedBy?: string;
};
