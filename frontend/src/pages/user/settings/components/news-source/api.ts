import { request } from '@umijs/max';

/** 与 server/pkg/settings/key.go KeyTelegramNewsCollector 一致 */
export const TELEGRAM_NEWS_COLLECTOR_KEY = 'settings.news.collector';
/** 与 server/pkg/settings/key.go KeyNewsCollectorEnabled 一致 */
export const TELEGRAM_NEWS_COLLECTOR_ENABLED_KEY = 'settings.news.collector.enabled';

export type TelegramNewsCollectorStored = {
  enabled: boolean;
  app_id: string;
  app_hash: string;
  session: string;
};

type GqlEnvelope<T> = { data?: T; errors?: { message?: string }[] };

const GET_SETTINGS = `
  query NewsSourceGetSettings($keys: [String!]!) {
    GetSettings(keys: $keys) {
      key
      value
    }
  }
`;

const SET_SETTINGS = `
  mutation NewsSourceSetSettings($key: String!, $value: String!) {
    SetSettings(key: $key, value: $value)
  }
`;

const SEND_CODE = `
  mutation NewsSourceSendTelegramCode($input: SendTelegramCodeInput!) {
    SendTelegramCode(input: $input) {
      success
      message
      codeHash
      session
    }
  }
`;

const GET_SESSION = `
  mutation NewsSourceGetTelegramSession($input: GetTelegramSessionInput!) {
    GetTelegramSession(input: $input) {
      success
      message
      session
    }
  }
`;

export function parseNewsCollectorValue(raw: string | undefined): TelegramNewsCollectorStored {
  const empty = { enabled: true, app_id: '', app_hash: '', session: '' };
  if (!raw || !raw.trim()) {
    return empty;
  }
  try {
    const o = JSON.parse(raw) as Record<string, unknown>;
    return {
      enabled: true,
      app_id: typeof o.app_id === 'string' ? o.app_id : '',
      app_hash: typeof o.app_hash === 'string' ? o.app_hash : '',
      session: typeof o.session === 'string' ? o.session : '',
    };
  } catch {
    return empty;
  }
}

function parseNewsCollectorEnabled(raw: string | undefined): boolean {
  if (!raw) {
    return true;
  }
  const v = raw.trim().toLowerCase();
  if (v === '' || v === 'true' || v === '1' || v === 'yes' || v === 'on') {
    return true;
  }
  if (v === 'false' || v === '0' || v === 'no' || v === 'off') {
    return false;
  }
  return true;
}

export async function fetchNewsCollectorConfig(): Promise<TelegramNewsCollectorStored> {
  const response = await request<
    GqlEnvelope<{ GetSettings?: { key: string; value: string }[] }>
  >('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: GET_SETTINGS,
      variables: { keys: [TELEGRAM_NEWS_COLLECTOR_KEY, TELEGRAM_NEWS_COLLECTOR_ENABLED_KEY] },
    }),
  });
  if (response.errors?.length) {
    throw new Error(response.errors[0]?.message || '加载配置失败');
  }
  const rows = response.data?.GetSettings ?? [];
  const collector = rows.find((e) => e.key === TELEGRAM_NEWS_COLLECTOR_KEY);
  const enabled = rows.find((e) => e.key === TELEGRAM_NEWS_COLLECTOR_ENABLED_KEY);
  const cfg = parseNewsCollectorValue(collector?.value);
  cfg.enabled = parseNewsCollectorEnabled(enabled?.value);
  return cfg;
}

export async function saveNewsCollectorConfig(cfg: TelegramNewsCollectorStored): Promise<void> {
  const trimmed = {
    app_id: cfg.app_id.trim(),
    app_hash: cfg.app_hash.trim(),
    session: cfg.session.trim(),
  };
  const responseConfig = await request<GqlEnvelope<{ SetSettings?: boolean }>>('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: SET_SETTINGS,
      variables: { key: TELEGRAM_NEWS_COLLECTOR_KEY, value: JSON.stringify(trimmed) },
    }),
  });
  if (responseConfig.errors?.length) {
    throw new Error(responseConfig.errors[0]?.message || '保存失败');
  }
  if (responseConfig.data?.SetSettings !== true) {
    throw new Error('保存失败');
  }

  const responseEnabled = await request<GqlEnvelope<{ SetSettings?: boolean }>>('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: SET_SETTINGS,
      variables: {
        key: TELEGRAM_NEWS_COLLECTOR_ENABLED_KEY,
        value: cfg.enabled ? 'true' : 'false',
      },
    }),
  });
  if (responseEnabled.errors?.length) {
    throw new Error(responseEnabled.errors[0]?.message || '保存失败');
  }
  if (responseEnabled.data?.SetSettings !== true) {
    throw new Error('保存失败');
  }
}

export async function sendTelegramCode(
  appId: string,
  appHash: string,
  phoneNumber: string,
): Promise<{ success: boolean; message?: string; codeHash?: string; session?: string }> {
  const response = await request<
    GqlEnvelope<{
      SendTelegramCode?: {
        success: boolean;
        message: string;
        codeHash: string;
        session: string;
      };
    }>
  >('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: SEND_CODE,
      variables: { input: { appId, appHash, phoneNumber } },
    }),
  });
  if (response.errors?.length) {
    throw new Error(response.errors[0]?.message || '发送验证码失败');
  }
  const p = response.data?.SendTelegramCode;
  if (!p) {
    throw new Error('发送验证码失败');
  }
  return {
    success: p.success,
    message: p.message,
    codeHash: p.codeHash,
    session: p.session,
  };
}

export async function getTelegramSession(
  appId: string,
  appHash: string,
  phoneNumber: string,
  codeHash: string,
  code: string,
  session: string,
): Promise<{ success: boolean; message?: string; session?: string }> {
  const response = await request<
    GqlEnvelope<{
      GetTelegramSession?: { success: boolean; message: string; session: string };
    }>
  >('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: GET_SESSION,
      variables: {
        input: { appId, appHash, phoneNumber, codeHash, code, session },
      },
    }),
  });
  if (response.errors?.length) {
    throw new Error(response.errors[0]?.message || '验证失败');
  }
  const p = response.data?.GetTelegramSession;
  if (!p) {
    throw new Error('验证失败');
  }
  return { success: p.success, message: p.message, session: p.session };
}
