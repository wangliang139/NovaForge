import { useEffect, useRef, useState } from 'react';
import {
  useSubscription,
  SubscriptionHookOptions,
  DocumentNode,
} from '@apollo/client';

export interface UseReconnectSubscriptionOptions<T>
  extends Omit<SubscriptionHookOptions<T>, 'skip'> {
  /** 检测间隔（ms），周期性检查连接是否超时，默认 5000 */
  checkInterval?: number;
  /** 超时时间（ms），超过此时间未收到数据则触发重连，默认 30000 */
  timeout?: number;
  /** 是否跳过订阅（与 useSubscription 的 skip 一致） */
  skip?: boolean;
  /** 重连成功后调用的回调函数 */
  onReconnectSuccess?: () => void;
}

/**
 * 基于 Apollo useSubscription 封装的自动重连订阅 Hook。
 * 通过周期性检测（checkInterval）判断是否在超时时间（timeout）内未收到数据，
 * 若超时则自动取消并重新订阅以触发重连。
 */
export function useSubscriptionWithReconnect<T>(
  query: DocumentNode,
  options: UseReconnectSubscriptionOptions<T> = {},
): ReturnType<typeof useSubscription<T>> {
  const {
    checkInterval = 5000,
    timeout = 30000,
    onData,
    onReconnectSuccess,
    skip: userSkip = false,
    ...rest
  } = options;

  const lastDataAtRef = useRef<number>(Date.now());
  const isPostReconnectRef = useRef(false);
  const [reconnectSkip, setReconnectSkip] = useState(false);
  const effectiveSkip = userSkip || reconnectSkip;

  const result = useSubscription<T>(query, {
    ...rest,
    skip: effectiveSkip,
    onData: (ctx) => {
      lastDataAtRef.current = Date.now();
      if (isPostReconnectRef.current) {
        isPostReconnectRef.current = false;
        onReconnectSuccess?.();
      }
      onData?.(ctx);
    },
  });

  useEffect(() => {
    if (userSkip || timeout <= 0 || checkInterval <= 0) return;

    const timer = setInterval(() => {
      const elapsed = Date.now() - lastDataAtRef.current;
      if (elapsed >= timeout) {
        setReconnectSkip(true);
        window.setTimeout(() => {
          lastDataAtRef.current = Date.now();
          isPostReconnectRef.current = true;
          setReconnectSkip(false);
        }, 200);
      }
    }, checkInterval);

    return () => clearInterval(timer);
  }, [userSkip, checkInterval, timeout]);

  return result;
}
