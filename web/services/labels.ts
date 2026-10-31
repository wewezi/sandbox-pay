import type { AdminTxStatus } from '../api-types';

/**
 * 订单状态 → [展示名, badge 修饰类]。
 *
 * 订单列表和数据看板的「最近订单」共用这张表：同一个状态在两个页面
 * 叫两个名字（或者一个带颜色一个不带）比重复本身更糟。
 */
export const STATUS_LABEL: Readonly<Record<AdminTxStatus, readonly [string, string]>> = {
  PENDING: ['待支付', 'warn'], SUCCESS: ['已支付', 'ok'], FAILED: ['失败', 'err'],
  REFUNDING: ['退款中', 'info'], REFUNDED: ['已退款', ''],
};
