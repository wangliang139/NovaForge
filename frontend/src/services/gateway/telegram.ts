import { request } from '@umijs/max';

export type PushConfigRecord = {
  provider: string;
  telegramBotToken: string;
  telegramChannelId: string;
  pushDocumentEnabled: boolean;
  pushAlarmEnabled: boolean;
  pushTradeEnabled: boolean;
  feishuWebhookUrl: string;
  feishuSecret: string;
  feishuKeyword: string;
};

const QUERY = `
  query PushConfigQuery {
    PushConfig {
      provider
      telegramBotToken
      telegramChannelId
      pushDocumentEnabled
      pushAlarmEnabled
      pushTradeEnabled
      feishuWebhookUrl
      feishuSecret
      feishuKeyword
    }
  }
`;

const MUTATION = `
  mutation UpdatePushConfig($input: UpdatePushConfigInput!) {
    UpdatePushConfig(input: $input) {
      provider
      telegramBotToken
      telegramChannelId
      pushDocumentEnabled
      pushAlarmEnabled
      pushTradeEnabled
      feishuWebhookUrl
      feishuSecret
      feishuKeyword
    }
  }
`;

type GqlEnvelope<T> = { data?: T; errors?: { message?: string }[] };

function normalizeRow(row: PushConfigRecord): PushConfigRecord {
  return {
    provider: row.provider ?? 'telegram',
    telegramBotToken: row.telegramBotToken ?? '',
    telegramChannelId: row.telegramChannelId ?? '',
    pushDocumentEnabled: row.pushDocumentEnabled ?? true,
    pushAlarmEnabled: row.pushAlarmEnabled ?? true,
    pushTradeEnabled: row.pushTradeEnabled ?? true,
    feishuWebhookUrl: row.feishuWebhookUrl ?? '',
    feishuSecret: row.feishuSecret ?? '',
    feishuKeyword: row.feishuKeyword ?? '',
  };
}

export async function queryPushConfig(): Promise<PushConfigRecord> {
  const response = await request<GqlEnvelope<{ PushConfig?: PushConfigRecord }>>('/query', {
    method: 'POST',
    data: JSON.stringify({ query: QUERY }),
  });
  if (response.errors?.length) {
    throw new Error(response.errors[0]?.message || '加载推送配置失败');
  }
  const row = response.data?.PushConfig;
  if (!row) {
    throw new Error('推送配置为空');
  }
  return normalizeRow(row);
}

export async function updatePushConfig(input: PushConfigRecord): Promise<PushConfigRecord> {
  const response = await request<GqlEnvelope<{ UpdatePushConfig?: PushConfigRecord }>>('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: MUTATION,
      variables: { input },
    }),
  });
  if (response.errors?.length) {
    throw new Error(response.errors[0]?.message || '保存失败');
  }
  const row = response.data?.UpdatePushConfig;
  if (!row) {
    throw new Error('保存后无返回数据');
  }
  return normalizeRow(row);
}
