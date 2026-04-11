import { request } from '@umijs/max';

/** 与 server/pkg/settings/key.go KeyTavilyAPIKey 一致 */
export const TAVILY_API_KEY_SETTING = 'settings.llm.tavily_api_key';

type GqlEnvelope<T> = { data?: T; errors?: { message?: string }[] };

const GET_SETTINGS = `
  query LlmToolsGetSettings($keys: [String!]!) {
    GetSettings(keys: $keys) {
      key
      value
    }
  }
`;

const SET_SETTINGS = `
  mutation LlmToolsSetSettings($key: String!, $value: String!) {
    SetSettings(key: $key, value: $value)
  }
`;

export type LlmToolsConfig = {
  tavilyApiKey: string;
};

export async function fetchTavilyConfig(): Promise<LlmToolsConfig> {
  const response = await request<
    GqlEnvelope<{ GetSettings?: { key: string; value: string }[] }>
  >('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: GET_SETTINGS,
      variables: { keys: [TAVILY_API_KEY_SETTING] },
    }),
  });
  if (response.errors?.length) {
    throw new Error(response.errors[0]?.message || '加载配置失败');
  }
  const row = response.data?.GetSettings?.find((e) => e.key === TAVILY_API_KEY_SETTING);
  return {
    tavilyApiKey: row?.value?.trim() ?? '',
  };
}

export async function saveTavilyConfig(cfg: LlmToolsConfig): Promise<void> {
  const value = cfg.tavilyApiKey.trim();
  const response = await request<GqlEnvelope<{ SetSettings?: boolean }>>('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: SET_SETTINGS,
      variables: { key: TAVILY_API_KEY_SETTING, value },
    }),
  });
  if (response.errors?.length) {
    throw new Error(response.errors[0]?.message || '保存失败');
  }
  if (response.data?.SetSettings !== true) {
    throw new Error('保存失败');
  }
}

