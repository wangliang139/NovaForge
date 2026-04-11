import { useRef } from 'react';
import { useSubscription, SubscriptionHookOptions, DocumentNode } from '@apollo/client';

export function useThrottledSubscription<T>(
  query: DocumentNode,
  options: SubscriptionHookOptions<T> & { throttleMs?: number },
) {
  const lastAtRef = useRef(0);
  const { throttleMs = 120, onData, ...rest } = options;

  return useSubscription<T>(query, {
    ...rest,
    ignoreResults: true,
    onData: (ctx) => {
      const now = Date.now();
      if (now - lastAtRef.current < throttleMs) return;
      lastAtRef.current = now;
      onData?.(ctx);
    },
  });
}
