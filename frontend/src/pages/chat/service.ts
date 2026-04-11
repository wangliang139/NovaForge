import { request } from '@umijs/max';
import { getAccessToken } from '@/utils/auth';
import type { ChatDeltaEvent, ChatModelOption, ChatSession, ChatSessionDetail } from './types';

const authHeaders = () => {
  const token = getAccessToken();
  const headers: Record<string, string> = {};
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }
  return headers;
};

export async function listChatSessions(): Promise<ChatSession[]> {
  const response = (await request('/api/chat/sessions', {
    method: 'GET',
    headers: authHeaders(),
  })) as { data: ChatSession[] };
  return response?.data || [];
}

export async function getChatSession(sessionId: string): Promise<ChatSessionDetail> {
  const response = (await request(`/api/chat/sessions/${sessionId}`, {
    method: 'GET',
    headers: authHeaders(),
  })) as { data: ChatSessionDetail };
  return response.data;
}

export async function deleteChatSession(sessionId: string): Promise<void> {
  await request(`/api/chat/sessions/${sessionId}`, {
    method: 'DELETE',
    headers: authHeaders(),
  });
}

export async function generateSessionTitleByFirstTurn(sessionId: string): Promise<string> {
  const response = (await request(`/api/chat/sessions/${sessionId}/title/generate`, {
    method: 'POST',
    headers: authHeaders(),
  })) as { data?: { title?: string } };
  return response?.data?.title || '';
}

export async function updateChatSessionTitle(sessionId: string, title: string): Promise<string> {
  const response = (await request(`/api/chat/sessions/${sessionId}/title`, {
    method: 'PATCH',
    headers: authHeaders(),
    data: { title },
  })) as { data?: { title?: string } };
  return response?.data?.title || '';
}

export async function listChatModels(): Promise<ChatModelOption[]> {
  const response = (await request('/api/chat/models', {
    method: 'GET',
    headers: authHeaders(),
  })) as { data: ChatModelOption[] };
  return response?.data || [];
}

export type UnifiedStreamBody = {
  sessionId?: string;
  dialogId?: string;
  regenerate?: boolean;
  content?: string;
  model?: string;
};

/** 统一流式对话：首包 type=ready 含 sessionId/dialogId，随后与原先 SSE 一致 */
export async function openUnifiedChatStream(
  body: UnifiedStreamBody,
  onEvent: (event: ChatDeltaEvent) => void | Promise<void>,
  signal?: AbortSignal,
) {
  const response = await fetch(`${API_URL}/api/chat/stream`, {
    method: 'POST',
    headers: {
      ...authHeaders(),
      'Content-Type': 'application/json',
      Accept: 'text/event-stream',
    },
    body: JSON.stringify({
      ...(body.sessionId ? { sessionId: body.sessionId } : {}),
      ...(body.dialogId ? { dialogId: body.dialogId } : {}),
      regenerate: !!body.regenerate,
      ...(body.content !== undefined && body.content !== '' ? { content: body.content } : {}),
      ...(body.model ? { model: body.model } : {}),
    }),
    signal,
  });

  if (!response.ok || !response.body) {
    const text = await response.text().catch(() => '');
    let msg = `stream failed: ${response.status}`;
    try {
      const j = JSON.parse(text) as { error?: string };
      if (j?.error) {
        msg = j.error;
      }
    } catch {
      if (text) {
        msg = text;
      }
    }
    throw new Error(msg);
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';

  while (true) {
    const { done, value } = await reader.read();
    if (done) {
      break;
    }
    buffer += decoder.decode(value, { stream: true });
    const chunks = buffer.split('\n\n');
    buffer = chunks.pop() || '';

    for (const chunk of chunks) {
      const lines = chunk.split('\n');
      const dataLines = lines.filter((line) => line.startsWith('data:')).map((line) => line.slice(5).trim());
      if (dataLines.length === 0) {
        continue;
      }
      const payload = dataLines.join('\n');
      try {
        const parsed = JSON.parse(payload) as ChatDeltaEvent;
        await onEvent(parsed);
      } catch (error) {
        console.error('failed to parse sse event', error, payload);
      }
    }
  }
}
