import { request } from '@umijs/max';

export type UserApiKeyPermission = 'query' | 'trade';

export type UserApiKeyRecord = {
  id: string;
  name: string;
  keyPrefix: string;
  permissions: UserApiKeyPermission[];
  /** Unix 毫秒时间戳 */
  createdAt: number;
};

const QUERY_USER_API_KEYS = `
  query UserApiKeysQuery {
    UserApiKeys {
      id
      name
      keyPrefix
      permissions
      createdAt
    }
  }
`;

const QUERY_NAME_AVAILABLE = `
  query UserApiKeyNameAvailableQuery($name: String!) {
    UserApiKeyNameAvailable(name: $name)
  }
`;

const MUTATION_CREATE = `
  mutation CreateUserApiKey($name: String!, $permissions: [UserApiKeyPermission!]!) {
    CreateUserApiKey(name: $name, permissions: $permissions) {
      record {
        id
        name
        keyPrefix
        permissions
        createdAt
      }
      plainSecret
    }
  }
`;

const MUTATION_DELETE = `
  mutation DeleteUserApiKey($id: ID!) {
    DeleteUserApiKey(id: $id)
  }
`;

type GqlEnvelope<T> = { data?: T; errors?: { message?: string }[] };

function normalizeApiKeyRow(row: UserApiKeyRecord): UserApiKeyRecord {
  const perms = Array.isArray(row.permissions) ? row.permissions : [];
  return {
    ...row,
    id: String(row.id),
    permissions: perms.filter((p): p is UserApiKeyPermission => p === 'query' || p === 'trade'),
  };
}

export async function queryUserApiKeys(): Promise<UserApiKeyRecord[]> {
  const response = await request<GqlEnvelope<{ UserApiKeys?: UserApiKeyRecord[] }>>('/query', {
    method: 'POST',
    data: JSON.stringify({ query: QUERY_USER_API_KEYS }),
  });
  if (response.errors?.length) {
    throw new Error(response.errors[0]?.message || '加载 API 密钥失败');
  }
  const raw = response.data?.UserApiKeys;
  if (!Array.isArray(raw)) {
    return [];
  }
  return raw.map(normalizeApiKeyRow);
}

export async function queryUserApiKeyNameAvailable(name: string): Promise<boolean> {
  const trimmed = name.trim();
  if (!trimmed) {
    return false;
  }
  const response = await request<GqlEnvelope<{ UserApiKeyNameAvailable?: boolean }>>('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_NAME_AVAILABLE,
      variables: { name: trimmed },
    }),
  });
  if (response.errors?.length) {
    throw new Error(response.errors[0]?.message || '校验名称失败');
  }
  return Boolean(response.data?.UserApiKeyNameAvailable);
}

export async function createUserApiKey(
  name: string,
  permissions: UserApiKeyPermission[],
): Promise<{ record: UserApiKeyRecord; plainSecret: string }> {
  const response = await request<
    GqlEnvelope<{
      CreateUserApiKey?: { record: UserApiKeyRecord; plainSecret: string };
    }>
  >('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: MUTATION_CREATE,
      variables: { name, permissions },
    }),
  });
  if (response.errors?.length) {
    throw new Error(response.errors[0]?.message || '创建失败');
  }
  const payload = response.data?.CreateUserApiKey;
  if (!payload?.plainSecret || !payload.record) {
    throw new Error('创建失败');
  }
  return {
    plainSecret: payload.plainSecret,
    record: normalizeApiKeyRow(payload.record),
  };
}

export async function deleteUserApiKey(id: string): Promise<boolean> {
  const response = await request<GqlEnvelope<{ DeleteUserApiKey?: boolean }>>('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: MUTATION_DELETE,
      variables: { id },
    }),
  });
  if (response.errors?.length) {
    throw new Error(response.errors[0]?.message || '删除失败');
  }
  return Boolean(response.data?.DeleteUserApiKey);
}
