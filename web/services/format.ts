const yuanFormat = new Intl.NumberFormat('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
export const yuan = (value: unknown) => '￥' + yuanFormat.format(Number(value) || 0);
const dateTimeFormat = new Intl.DateTimeFormat('zh-CN', {
  year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23',
});
export type DateValue = string | number | Date | null | undefined;
export function dateTime(value: DateValue) {
  if (!value) return '';
  const date = new Date(value);
  return isNaN(date.getTime()) ? String(value) : dateTimeFormat.format(date).replace(/\//g, '-');
}
const percentFormat = new Intl.NumberFormat('zh-CN', { style: 'percent', minimumFractionDigits: 1, maximumFractionDigits: 1 });
export const percent = (ratio: number) => percentFormat.format(ratio);
const countFormat = new Intl.NumberFormat('zh-CN');
export const count = (value: number) => countFormat.format(value);
const relativeFormat = new Intl.RelativeTimeFormat('zh-CN', { numeric: 'auto' });
const midnight = (date: Date) => new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
export function relTime(value: DateValue, now = new Date()) {
  if (!value) return '';
  const then = new Date(value);
  if (isNaN(then.getTime())) return '';
  const seconds = Math.round((then.getTime() - now.getTime()) / 1000);
  const abs = Math.abs(seconds);
  if (abs < 60) return relativeFormat.format(seconds, 'second');
  if (abs < 3600) return relativeFormat.format(Math.round(seconds / 60), 'minute');
  if (abs < 86400) return relativeFormat.format(Math.round(seconds / 3600), 'hour');
  return relativeFormat.format(Math.round((midnight(then) - midnight(now)) / 86400000), 'day');
}
export function mmss(seconds: number) {
  if (seconds <= 0) return null;
  return `${String(Math.floor(seconds / 60)).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}`;
}

export function qs(params: Record<string, string | number | undefined | null>) {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== null && value !== '') query.set(key, String(value));
  }
  const result = query.toString();
  return result ? '?' + result : '';
}
