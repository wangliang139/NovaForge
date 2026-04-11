export function getDecimalPrecision(str: string) {
  str = str.trim();
  if (str.includes('e')) {
    let [base, exponent] = str.split('e');
    let precision = (base.split('.')[1] || '').length - parseInt(exponent);
    return Math.max(0, precision);
  } else {
    let raw = str.split('.')[1] || '';
    raw = raw.replace(/0+$/, '');
    return raw.length;
  }
}

export function toSafeNumber(value: string | number | null | undefined): number {
  const rawStr = String(value ?? '')
    .replace(/,/g, '')
    .trim();
  if (!rawStr) return 0;
  const n = Number(rawStr);
  return Number.isFinite(n) ? n : 0;
}

export function formatByPrecision(
  value: string | number | null | undefined,
  precision?: number,
  empty: string = '--',
): string {
  if (value === null || value === undefined) return empty;
  const rawStr = String(value).replace(/,/g, '').trim();
  if (!rawStr) return empty;
  const n = Number(rawStr);
  if (!Number.isFinite(n)) return empty;
  if (!Number.isFinite(precision as number) || (precision as number) < 0) return rawStr;
  return n.toLocaleString("en-US", {
    useGrouping: false,      // 不加千分位
    maximumFractionDigits: precision as number // 最多8位
  });
}

function trimTrailingZeros(numText: string) {
  return numText.replace(/\.?0+$/, '');
}

export type FormatWanYiOptions = {
  digits?: number;
  empty?: string;
  space?: boolean;
};

export type FormatKMBOptions = {
  digits?: number;
  empty?: string;
};

export function formatWanYi(
  value: number | string | null | undefined,
  options: FormatWanYiOptions = {},
) {
  const empty = options.empty ?? '--';
  if (value === null || value === undefined) return empty;

  const raw = typeof value === 'number' ? value : Number(String(value).replace(/,/g, '').trim());
  if (!Number.isFinite(raw)) return empty;

  const abs = Math.abs(raw);
  const digits = options.digits ?? 2;
  const join = options.space === false ? '' : ' ';

  if (abs >= 100000000) {
    return `${trimTrailingZeros((raw / 100000000).toFixed(digits))}${join}亿`;
  }
  if (abs >= 10000) {
    return `${trimTrailingZeros((raw / 10000).toFixed(digits))}${join}万`;
  }
  return trimTrailingZeros(raw.toFixed(digits));
}

export function formatKMB(
  value: number | string | null | undefined,
  options: FormatKMBOptions = {},
) {
  const empty = options.empty ?? '--';
  if (value === null || value === undefined) return empty;

  const raw = typeof value === 'number' ? value : Number(String(value).replace(/,/g, '').trim());
  if (!Number.isFinite(raw)) return empty;

  const abs = Math.abs(raw);
  const digits = options.digits ?? 2;

  if (abs >= 1_000_000_000) return `${trimTrailingZeros((raw / 1_000_000_000).toFixed(digits))}B`;
  if (abs >= 1_000_000) return `${trimTrailingZeros((raw / 1_000_000).toFixed(digits))}M`;
  if (abs >= 1_000) return `${trimTrailingZeros((raw / 1_000).toFixed(digits))}K`;
  return trimTrailingZeros(raw.toFixed(digits));
}

export function digitalToPercent(digital: number | string, digits: number = 6) {
  if (digital === null || digital === undefined) return '--';
  const n =
    typeof digital === 'number' ? digital : Number(String(digital).replace(/,/g, '').trim());
  if (!Number.isFinite(n)) return '--';
  const percent = n * 100;
  const txt = percent.toFixed(digits).replace(/\.?0+$/, '');
  const sign = percent > 0 ? '+' : percent < 0 ? '' : '';
  return `${sign}${txt}%`;
}
