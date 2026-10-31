import type { GeoIPLocation } from '../api-types';

const placeholders = new Set(['0', '-', 'unknown', 'null', 'undefined', 'n/a', '未知']);

export function cleanLocationValue(value: string | null | undefined): string {
  const cleaned = value?.trim() ?? '';
  return placeholders.has(cleaned.toLowerCase()) ? '' : cleaned;
}

export function networkLabel(network: GeoIPLocation['network']): string {
  switch (network) {
    case 'public': return '公网';
    case 'private': return '内网';
    case 'loopback': return '本机';
    case 'link_local': return '链路本地';
    case 'reserved': return '保留地址';
    default: return '网络类型未解析';
  }
}

export function locationSummary(clientIp: string | null | undefined, location: GeoIPLocation | null | undefined): string {
  if (!clientIp) return '未记录';
  if (location?.network && !['public', 'unknown'].includes(location.network)) return networkLabel(location.network);
  const parts = [location?.country, location?.region, location?.city].map(cleanLocationValue).filter(Boolean);
  if (parts.length) return [...new Set(parts)].join(' · ');
  return '归属地未解析';
}
