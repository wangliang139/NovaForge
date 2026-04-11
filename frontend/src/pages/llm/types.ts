export type LlmConfig = {
  temperature?: number;
  topP?: number;
  maxTokens?: number;
  maxCompletionTokens?: number;
};

export type LlmMessage = {
  role: string;
  content: string;
  name?: string;
};

export enum PlatformType {
  // SILICONFLOW = 'siliconflow',
  OPENROUTER = 'openrouter',
}

export const LlmPlatformColor: Record<PlatformType, string> = {
  // [PlatformType.SILICONFLOW]: '#ffd666',
  [PlatformType.OPENROUTER]: '#69b1ff',
};

export enum LlmMessageRole {
  USER = 'user',
  ASSISTANT = 'assistant',
  SYSTEM = 'system',
  TOOL = 'tool',
}

export const LlmMessageRoleColor: Record<LlmMessageRole, string> = {
  [LlmMessageRole.USER]: '#fde3cf',
  [LlmMessageRole.ASSISTANT]: '#d3f261',
  [LlmMessageRole.SYSTEM]: '#ffd666',
  [LlmMessageRole.TOOL]: '#69b1ff',
};

export enum LlmResponseFormatType {
  TEXT = 'text',
  JSON_OBJECT = 'json_object',
  JSON_SCHEMA = 'json_schema',
}

export type LlmResponseFormatJsonSchema = {
  name: string;
  strict: boolean;
  schema: string;
};

export type LlmResponseFormat = {
  type: string;
  jsonSchema?: LlmResponseFormatJsonSchema;
};

export type LlmScene = {
  id: string;
  key: string;
  name: string;
  description: string;
  config: LlmConfig;
  messages: LlmMessage[];
  timeout: number;
  responseFormat: LlmResponseFormat;
  enabled: boolean;
  createdAt: number;
  updatedAt: number;
};

export type LlmPrompt = {
  id?: string;
  sceneId?: string;
  sceneKey?: string;
  platform?: string;
  name?: string;
  model?: string;
  providers?: string[];
  config?: LlmConfig;
  messages?: LlmMessage[];
  timeout?: number;
  weight?: number;
  variants?: string[];
  enabled?: boolean;
  createdAt?: number;
  updatedAt?: number;
};

export type LlmSceneWithPrompts = {
  scene: LlmScene;
  prompts: LlmPrompt[];
};

export type QueryLlmScenesParams = API.PageParams & {
  enabled?: boolean;
};

export type CompletionMetadata = {
  sceneKey: string;
  sceneId: string;
  promptId: string;
  model: string;
  provider: string;
};

export type CompletionUsage = {
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
};
