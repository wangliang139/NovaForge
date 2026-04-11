import { request } from '@umijs/max';

const MUTATION_CHANGE_PASSWORD = `
  mutation ChangeUserPassword($currentPassword: String!, $newPassword: String!) {
    ChangeUserPassword(currentPassword: $currentPassword, newPassword: $newPassword)
  }
`;

type GqlEnvelope<T> = { data?: T; errors?: { message?: string }[] };

/** 校验当前密码后更新为新密码（密码需为前端 RSA 加密后的 Base64，与登录一致） */
export async function changeUserPassword(params: {
  currentPassword: string;
  newPassword: string;
}): Promise<boolean> {
  const response = await request<GqlEnvelope<{ ChangeUserPassword?: boolean }>>('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: MUTATION_CHANGE_PASSWORD,
      variables: {
        currentPassword: params.currentPassword,
        newPassword: params.newPassword,
      },
    }),
  });
  if (response.errors?.length) {
    throw new Error(response.errors[0]?.message || '修改密码失败');
  }
  const ok = response.data?.ChangeUserPassword;
  if (ok !== true) {
    throw new Error('修改密码失败');
  }
  return true;
}
