import { useSubscriptionWithReconnect } from '@/hooks/useReconnectSubscription';
import {
  Document,
  DocumentStatus,
  GetSourceText,
  queryDocuments,
} from '@/services/gateway/document';
import { StreamEvent } from '@/services/gateway/market';
import { SUB_STREAM } from '@/services/gateway/subscription';
import { SoundOutlined } from '@ant-design/icons';
import { Row, theme } from 'antd';
import dayjs from 'dayjs';
import React, { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';

const NEW_FLASH_DURATION_MS = 3000;

const FLASH_TICK_MS = 400;

const AudioContextClass =
  typeof window !== 'undefined'
    ? (window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext)
    : null;

/** 播放新消息提示音（短促叮一声），需传入已解锁的 AudioContext */
function playNotificationSound(ctx: AudioContext | null) {
  if (!ctx || ctx.state === 'suspended') return;
  try {
    const osc = ctx.createOscillator();
    const gain = ctx.createGain();
    osc.connect(gain);
    gain.connect(ctx.destination);
    osc.type = 'sine';
    osc.frequency.setValueAtTime(880, ctx.currentTime);
    osc.frequency.setValueAtTime(660, ctx.currentTime + 0.08);
    gain.gain.setValueAtTime(0.12, ctx.currentTime);
    gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 0.2);
    osc.start(ctx.currentTime);
    osc.stop(ctx.currentTime + 0.2);
  } catch {
    // 静默忽略
  }
}

export const NewsTicker: React.FC = () => {
  const { token } = theme.useToken();
  const [latest, setLatest] = useState<Document | null>(null);
  const [isNewFlash, setIsNewFlash] = useState(false);
  const [flashVisible, setFlashVisible] = useState(true); // 闪烁时在亮/暗之间切换
  const [hideTicker, setHideTicker] = useState(false);
  const [tickerWidthPx, setTickerWidthPx] = useState<number>(0);
  const [showSummary, setShowSummary] = useState<boolean>(false);
  const [titleWidthPx, setTitleWidthPx] = useState<number>(0);
  const [summaryWidthPx, setSummaryWidthPx] = useState<number>(0);
  const flashTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const flashIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const audioCtxRef = useRef<AudioContext | null>(null);
  const rowRef = useRef<HTMLDivElement | null>(null);
  const wrapRef = useRef<HTMLDivElement | null>(null);
  const iconRef = useRef<HTMLSpanElement | null>(null);
  const metaRef = useRef<HTMLSpanElement | null>(null);
  const titleMeasureRef = useRef<HTMLSpanElement | null>(null);
  const summaryMeasureRef = useRef<HTMLSpanElement | null>(null);
  const metaMeasureRef = useRef<HTMLSpanElement | null>(null);
  const ellipsisMeasureRef = useRef<HTMLSpanElement | null>(null);

  // 浏览器要求“用户先与页面交互”才能播放声音，在首次点击/触摸时解锁
  useEffect(() => {
    if (!AudioContextClass) return;
    const unlock = () => {
      if (audioCtxRef.current) return;
      const ctx = new AudioContextClass();
      ctx.resume().then(() => {
        audioCtxRef.current = ctx;
      });
      document.removeEventListener('click', unlock);
      document.removeEventListener('keydown', unlock);
      document.removeEventListener('touchstart', unlock);
    };
    document.addEventListener('click', unlock);
    document.addEventListener('keydown', unlock);
    document.addEventListener('touchstart', unlock);
    return () => {
      document.removeEventListener('click', unlock);
      document.removeEventListener('keydown', unlock);
      document.removeEventListener('touchstart', unlock);
    };
  }, []);

  const clearFlashTimers = () => {
    if (flashTimerRef.current) {
      clearTimeout(flashTimerRef.current);
      flashTimerRef.current = null;
    }
    if (flashIntervalRef.current) {
      clearInterval(flashIntervalRef.current);
      flashIntervalRef.current = null;
    }
  };

  // 首次加载时查询一条最新 active 文档展示
  useEffect(() => {
    queryDocuments({
      status: DocumentStatus.ACTIVE,
      pageSize: 1,
      current: 1,
    }).then((res) => {
      const first = res?.list?.[0];
      // 如果第一条的发布时间超过1天，不展示
      if (first && first.publishedAt) {
        const now = dayjs();
        const published = dayjs.unix(first.publishedAt);
        if (now.diff(published, 'day') >= 1) {
          // 超过一天则认为无可展示内容
          return;
        }
      }
      if (first) setLatest(first);
    });
    return clearFlashTimers;
  }, []);

  useSubscriptionWithReconnect<{ Stream: StreamEvent }>(SUB_STREAM, {
    timeout: 3600_000,
    variables: {
      input: { type: 'social' },
    },
    onData: ({ data }) => {
      const event = data.data?.Stream;
      if (event?.type === 'social' && event.social) {
        playNotificationSound(audioCtxRef.current);
        clearFlashTimers();
        setLatest(event.social as unknown as Document);
        setFlashVisible(true);
        setIsNewFlash(true);
        flashIntervalRef.current = setInterval(() => {
          setFlashVisible((v) => !v);
        }, FLASH_TICK_MS);
        flashTimerRef.current = setTimeout(() => {
          clearFlashTimers();
          setIsNewFlash(false);
          setFlashVisible(true);
        }, NEW_FLASH_DURATION_MS);
      }
    },
  });

  const line = useMemo(() => {
    if (!latest) return null;
    const title = latest.aiTitle;
    const summary = latest.aiSummary ? ` · ${latest.aiSummary}` : '';
    const source = GetSourceText(latest.source);
    const time = latest.publishedAt
      ? dayjs.unix(latest.publishedAt).format('MM-DD HH:mm')
      : '';
    return { title, summary, source, time, url: latest.url || undefined };
  }, [latest]);

  // 1) 计算“上层可用宽度”
  // upperAvailable = headerWidth - leftLogoWidth - rightContentWidth
  // tickerWidth = upperUsable * 0.8 (upperUsable 需要减掉 centerRow padding)
  useLayoutEffect(() => {
    const scheduleRef: { raf: number | null; timer: number | null; tries: number } = {
      raf: null,
      timer: null,
      tries: 0,
    };

    const updateTickerWidth = (): boolean => {
      const headerEl = document.querySelector('.ant-pro-global-header') as HTMLElement | null;
      const logoEl = headerEl?.querySelector('.ant-pro-global-header-logo') as HTMLElement | null;
      const rightEl = headerEl?.querySelector(
        '.ant-pro-global-header-right-content',
      ) as HTMLElement | null;
      const centerRowEl = rowRef.current;

      if (!headerEl || !logoEl || !rightEl || !centerRowEl) return false;

      const headerW = headerEl.getBoundingClientRect().width;
      const logoW = logoEl.getBoundingClientRect().width;
      const rightW = rightEl.getBoundingClientRect().width;
      const upperAvailable = Math.max(0, headerW - logoW - rightW);

      const style = window.getComputedStyle(centerRowEl);
      const paddingL = parseFloat(style.paddingLeft || '0') || 0;
      const paddingR = parseFloat(style.paddingRight || '0') || 0;

      const usableForTicker = Math.max(0, upperAvailable - paddingL - paddingR);
      const nextTickerWidth = usableForTicker * 0.8;

      setTickerWidthPx((prev) => {
        // 避免微小抖动导致频繁渲染
        if (Math.abs(prev - nextTickerWidth) < 1) return prev;
        return nextTickerWidth;
      });
      return true;
    };

    const scheduleUpdate = () => {
      if (scheduleRef.raf != null) return;
      scheduleRef.raf = window.requestAnimationFrame(() => {
        scheduleRef.raf = null;
        const ok = updateTickerWidth();
        // 刷新首屏时 DOM 可能还没挂载齐；短暂重试避免“必须触发一次 resize 才显示”
        if (!ok && scheduleRef.tries < 60 && scheduleRef.timer == null) {
          scheduleRef.tries += 1;
          scheduleRef.timer = window.setTimeout(() => {
            scheduleRef.timer = null;
            scheduleUpdate();
          }, 50);
        }
      });
    };

    scheduleUpdate();
    window.addEventListener('resize', scheduleUpdate);

    let headerEl: HTMLElement | null = null;
    try {
      headerEl = document.querySelector('.ant-pro-global-header') as HTMLElement | null;
    } catch {
      headerEl = null;
    }

    const ro = headerEl
      ? new ResizeObserver(() => {
          scheduleUpdate();
        })
      : null;
    if (ro && headerEl) ro.observe(headerEl);

    return () => {
      window.removeEventListener('resize', scheduleUpdate);
      if (ro && headerEl) ro.disconnect();
      if (scheduleRef.raf != null) window.cancelAnimationFrame(scheduleRef.raf);
      if (scheduleRef.timer != null) window.clearTimeout(scheduleRef.timer);
    };
  }, []);

  // 2) 根据 tickerWidthPx + title/summary/meta 的自然宽度，计算：
  // - summary 优先缩略
  // - title 不够显示时，隐藏整个 NewsTicker
  useLayoutEffect(() => {
    if (!line) return;
    if (!tickerWidthPx || tickerWidthPx <= 0) return;

    const iconW = iconRef.current?.getBoundingClientRect().width ?? 0;
    const gap = 8; // icon 的 marginRight
    const metaNaturalW = metaMeasureRef.current?.getBoundingClientRect().width ?? 0;
    const titleNaturalW = titleMeasureRef.current?.getBoundingClientRect().width ?? 0;
    const summaryNaturalW = line.summary
      ? summaryMeasureRef.current?.getBoundingClientRect().width ?? 0
      : 0;
    const ellipsisW = ellipsisMeasureRef.current?.getBoundingClientRect().width ?? 0;

    const availableForLeft = Math.max(0, tickerWidthPx - iconW - gap - metaNaturalW);

    if (!ellipsisW || availableForLeft < ellipsisW + 1) {
      setHideTicker(true);
      setShowSummary(false);
      setTitleWidthPx(Math.max(0, availableForLeft));
      setSummaryWidthPx(0);
      return;
    }

    setHideTicker(false);

    // 没有 summary：只考虑 title
    if (!line.summary) {
      setShowSummary(false);
      setSummaryWidthPx(0);
      setTitleWidthPx(Math.max(0, Math.min(titleNaturalW, availableForLeft)));
      return;
    }

    // case1：title + summary 都能完整展示
    if (availableForLeft >= titleNaturalW + summaryNaturalW) {
      setShowSummary(true);
      setTitleWidthPx(titleNaturalW);
      setSummaryWidthPx(summaryNaturalW);
      return;
    }

    // case2：title 能完整展示，summary 通过 ellipsis 缩略
    if (availableForLeft >= titleNaturalW) {
      setShowSummary(true);
      setTitleWidthPx(titleNaturalW);
      setSummaryWidthPx(Math.max(0, availableForLeft - titleNaturalW));
      return;
    }

    // case3：title 都完整不了 => summary 先消失，title 通过 ellipsis
    setShowSummary(false);
    setSummaryWidthPx(0);
    setTitleWidthPx(availableForLeft);

    // title 也不足以形成“可见的省略号” => 隐藏整个 ticker
    if (availableForLeft < ellipsisW + 1) {
      setHideTicker(true);
    }
  }, [line, tickerWidthPx]);

  if (!line) return <></>;
  // tickerWidthPx 还没算出来时先隐藏，避免出现空白/布局抖动
  const outerVisibility = hideTicker || tickerWidthPx <= 0 ? 'hidden' : 'visible';

  const content = (
    <span style={{ display: 'flex', alignItems: 'baseline', minWidth: 0, flex: '1 1 auto' }}>
      <span style={{ display: 'flex', alignItems: 'baseline', minWidth: 0, flex: '0 0 auto' }}>
        <span
          style={{
            display: 'inline-block',
            width: titleWidthPx > 0 ? `${Math.max(0, titleWidthPx)}px` : '100%',
            minWidth: 0,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            flex: '0 0 auto',
            fontWeight: 400,
            color: token.colorText,
          }}
        >
          {line.title}
        </span>
        {showSummary && line.summary && (
          <span
            style={{
              display: 'inline-block',
              width: summaryWidthPx > 0 ? `${Math.max(0, summaryWidthPx)}px` : '100%',
              minWidth: 0,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              flex: '0 0 auto',
              color: token.colorTextSecondary,
            }}
          >
            {line.summary}
          </span>
        )}
      </span>
      <span
        ref={metaRef}
        style={{
          flexShrink: 0,
          marginLeft: 'auto',
          paddingLeft: '8px',
          color: token.colorTextTertiary,
          whiteSpace: 'nowrap',
        }}
      >
        {line.source} · {line.time}
      </span>
    </span>
  );

  const iconOpacity = isNewFlash ? (flashVisible ? 1 : 0.35) : 1;

  return (
    <Row
      style={{ width: '100%', visibility: outerVisibility }}
      align="middle"
      justify="center"
      wrap={false}
      ref={rowRef}
    >
      <div
        ref={wrapRef}
        style={{
          width: tickerWidthPx ? `${tickerWidthPx}px` : '80%',
          display: 'flex',
          alignItems: 'center',
          minWidth: 0,
          overflow: 'hidden',
        }}
      >
        <span
          ref={iconRef}
          style={{
            marginRight: '8px',
            color: token.colorWarning,
            opacity: iconOpacity,
            transition: 'opacity 0.15s ease-out',
            flexShrink: 0,
          }}
          aria-hidden
        >
          <SoundOutlined style={{ fontSize: '14px' }} />
        </span>
        <div
          style={{
            minWidth: 0,
            margin: 0,
            overflow: 'hidden',
            fontSize: '13px',
            lineHeight: '22px',
            flex: '1 1 auto',
          }}
        >
          {line.url ? (
            <a
              href={line.url}
              target="_blank"
              rel="noopener noreferrer"
              style={{
                width: '100%',
                textDecoration: 'none',
                display: 'flex',
                minWidth: 0,
                overflow: 'hidden',
                color: token.colorText,
              }}
            >
              {content}
            </a>
          ) : (
            <span style={{ width: '100%', display: 'flex', minWidth: 0, overflow: 'hidden' }}>
              {content}
            </span>
          )}
        </div>
      </div>
      {/* 自然宽度测量用（不参与布局） */}
      <div
        style={{
          position: 'absolute',
          visibility: 'hidden',
          left: -9999,
          top: 0,
          whiteSpace: 'nowrap',
          fontSize: '13px',
          lineHeight: '22px',
          pointerEvents: 'none',
        }}
      >
        <span ref={titleMeasureRef} style={{ fontWeight: 400, color: token.colorText }}>
          {line.title}
        </span>
        <span ref={summaryMeasureRef} style={{ fontWeight: 400, color: token.colorTextSecondary }}>
          {line.summary ?? ''}
        </span>
        <span ref={metaMeasureRef} style={{ paddingLeft: '8px', color: token.colorTextTertiary }}>
          {line.source} · {line.time}
        </span>
        <span ref={ellipsisMeasureRef} style={{ fontWeight: 400, color: token.colorText }}>
          …
        </span>
      </div>
    </Row>
  );
};
