import { useEffect, useRef, useState } from 'react';

// 发送消息类型定义
type Message = {
  id: number;
  op: string;
  args: string[];
};

// 接收消息类型定义
type WsEvent = {
  type: number;
  id?: number;
  success?: boolean;
  stream?: string;
  data?: any;
};

// WebSocketOptions 类型定义，包括可选参数和默认值
type WebSocketOptions = {
  url: string;
  onOpen?: () => void;
  onClose?: (event: CloseEvent) => void;
  onError?: (event: Event) => void;
  onMessage?: (message: WsEvent) => void;
  reconnectInterval?: number;
  reconnectAttempts?: number;
};

const defaultOptions: Required<WebSocketOptions> = {
  url: '',
  onOpen: () => {},
  onClose: () => {},
  onError: () => {},
  onMessage: () => {},
  reconnectInterval: 1000,
  reconnectAttempts: Number.MAX_VALUE,
};

const useWebSocket = (
  options: WebSocketOptions,
): [WebSocket | undefined, (message: Message) => void, boolean, () => void] => {
  const { url, onOpen, onClose, onError, onMessage, reconnectInterval, reconnectAttempts } = {
    ...defaultOptions,
    ...options,
  };

  const [isConnected, setIsConnected] = useState(false);
  const [reconnectCount, setReconnectCount] = useState(0);
  const socketRef = useRef<WebSocket>();
  const reconnectTimerRef = useRef<NodeJS.Timeout>();

  const handleOpen = () => {
    setIsConnected(true);
    setReconnectCount(0);
    onOpen?.();
  };

  const handleClose = (event: CloseEvent) => {
    setIsConnected(false);
    onClose?.(event);
    if (reconnectCount < reconnectAttempts) {
      const delay = Math.min(reconnectInterval * 2 ** reconnectCount, 30_000);
      reconnectTimerRef.current = setTimeout(() => {
        setReconnectCount((c) => c + 1);
        connect();
      }, delay);
    }
  };

  const handleError = (event: Event) => {
    onError?.(event);
  };

  const handleMessage = (event: MessageEvent) => {
    const message = JSON.parse(event.data) as WsEvent;
    onMessage?.(message);
  };

  const connect = () => {
    setIsConnected(false);
    const socket = new WebSocket(url);

    socket.onopen = handleOpen;
    socket.onclose = handleClose;
    socket.onerror = handleError;
    socket.onmessage = handleMessage;

    socketRef.current = socket;
  };

  const close = () => {
    socketRef.current?.close();
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
    }
  };

  useEffect(() => {
    connect();
    return () => {
      close();
    };
  }, [url]); // Adding `url` as a dependency to reconnect if the URL changes

  const sendMessage = (message: Message) => {
    if (isConnected && socketRef.current) {
      try {
        socketRef.current.send(JSON.stringify(message));
      } catch (e) {
        console.error('Error sending message:', e);
      }
    } else {
      console.error('Cannot send message - WebSocket is not connected');
    }
  };

  return [socketRef.current, sendMessage, isConnected, close];
};

export default useWebSocket;
