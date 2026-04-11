import { request } from '@umijs/max';

export type LlmProviderConfigRecord = {
  openRouterApiKey: string;
  siliconFlowApiKey: string;
  defaultModel: string;
};

const QUERY = `
  query LlmProviderConfigQuery {
    LlmProviderConfig {
      openRouterApiKey
      siliconFlowApiKey
      defaultModel
    }
  }
`;

const MUTATION = `
  mutation UpdateLlmProviderConfig($input: UpdateLlmProviderConfigInput!) {
    UpdateLlmProviderConfig(input: $input) {
      openRouterApiKey
      siliconFlowApiKey
      defaultModel
    }
  }
`;

type GqlEnvelope<T> = { data?: T; errors?: { message?: string }[] };

function normalizeRow(row: LlmProviderConfigRecord): LlmProviderConfigRecord {
  return {
    openRouterApiKey: row.openRouterApiKey ?? '',
    siliconFlowApiKey: row.siliconFlowApiKey ?? '',
    defaultModel: row.defaultModel ?? '',
  };
}

export async function queryLlmProviderConfig(): Promise<LlmProviderConfigRecord> {
  const response = await request<GqlEnvelope<{ LlmProviderConfig?: LlmProviderConfigRecord }>>('/query', {
    method: 'POST',
    data: JSON.stringify({ query: QUERY }),
  });
  if (response.errors?.length) {
    throw new Error(response.errors[0]?.message || '加载大模型配置失败');
  }
  const row = response.data?.LlmProviderConfig;
  if (!row) {
    throw new Error('大模型配置为空');
  }
  return normalizeRow(row);
}

export async function updateLlmProviderConfig(
  input: LlmProviderConfigRecord,
): Promise<LlmProviderConfigRecord> {
  const response = await request<GqlEnvelope<{ UpdateLlmProviderConfig?: LlmProviderConfigRecord }>>('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: MUTATION,
      variables: { input },
    }),
  });
  if (response.errors?.length) {
    throw new Error(response.errors[0]?.message || '保存失败');
  }
  const row = response.data?.UpdateLlmProviderConfig;
  if (!row) {
    throw new Error('保存后无返回数据');
  }
  return normalizeRow(row);
}
