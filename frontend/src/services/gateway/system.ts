import { request } from '@umijs/max';

const MUTATION = `
  mutation RequestGatewayRestart {
    RequestGatewayRestart
  }
`;

type GqlEnvelope<T> = { data?: T; errors?: { message?: string }[] };

export async function requestGatewayRestart(): Promise<boolean> {
  const response = await request<GqlEnvelope<{ RequestGatewayRestart?: boolean }>>('/query', {
    method: 'POST',
    data: JSON.stringify({ query: MUTATION }),
  });
  if (response.errors?.length) {
    throw new Error(response.errors[0]?.message || '请求重启失败');
  }
  if (response.data?.RequestGatewayRestart !== true) {
    throw new Error('服务器未接受重启请求');
  }
  return true;
}
