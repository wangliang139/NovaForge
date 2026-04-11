import { request } from '@umijs/max';

export type NetworkProxyConfigRecord = {
  httpProxyUrl: string;
};

const QUERY = `
  query NetworkProxyConfigQuery {
    NetworkProxyConfig {
      httpProxyUrl
    }
  }
`;

const MUTATION = `
  mutation UpdateNetworkProxyConfig($input: UpdateNetworkProxyConfigInput!) {
    UpdateNetworkProxyConfig(input: $input) {
      httpProxyUrl
    }
  }
`;

type GqlEnvelope<T> = { data?: T; errors?: { message?: string }[] };

function normalizeRow(row: NetworkProxyConfigRecord): NetworkProxyConfigRecord {
  return {
    httpProxyUrl: row.httpProxyUrl ?? '',
  };
}

export async function queryNetworkProxyConfig(): Promise<NetworkProxyConfigRecord> {
  const response = await request<GqlEnvelope<{ NetworkProxyConfig?: NetworkProxyConfigRecord }>>('/query', {
    method: 'POST',
    data: JSON.stringify({ query: QUERY }),
  });
  if (response.errors?.length) {
    throw new Error(response.errors[0]?.message || '加载代理配置失败');
  }
  const row = response.data?.NetworkProxyConfig;
  if (!row) {
    throw new Error('代理配置为空');
  }
  return normalizeRow(row);
}

export async function updateNetworkProxyConfig(
  input: NetworkProxyConfigRecord,
): Promise<NetworkProxyConfigRecord> {
  const response = await request<
    GqlEnvelope<{ UpdateNetworkProxyConfig?: NetworkProxyConfigRecord }>
  >('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: MUTATION,
      variables: { input },
    }),
  });
  if (response.errors?.length) {
    throw new Error(response.errors[0]?.message || '保存失败');
  }
  const row = response.data?.UpdateNetworkProxyConfig;
  if (!row) {
    throw new Error('保存后无返回数据');
  }
  return normalizeRow(row);
}
