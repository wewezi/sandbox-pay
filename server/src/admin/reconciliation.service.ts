import { Injectable, Logger } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { Between, Repository } from 'typeorm';
import { parse as parseCsvSync } from 'csv-parse/sync';
import { PaymentOrder, OrderStatus } from '../entities/payment-order.entity';
import { ReconciliationRecord, ReconStatus } from '../entities/reconciliation-record.entity';
import { yuanStringToCents, toYuan } from '../common/money';

/**
 * A normalised row representing one line from an upstream PSP's daily bill.
 *
 * All money fields are integer minor units to match local storage. The
 * `parseCsv*` helpers below convert raw CSV strings to this shape.
 */
export interface BillRow {
  orderNo: string;              // merchant out_trade_no
  upstreamTradeNo: string;      // PSP-side trade no
  amountCents: number;          // gross paid amount (cents)
  feeCents: number;             // upstream PSP fee (cents)
}

@Injectable()
export class ReconciliationService {
  private readonly logger = new Logger(ReconciliationService.name);

  constructor(
    @InjectRepository(PaymentOrder)
    private readonly orderRepository: Repository<PaymentOrder>,
    @InjectRepository(ReconciliationRecord)
    private readonly reconRepository: Repository<ReconciliationRecord>,
  ) {}

  /**
   * Parse an Alipay-style bill CSV (header row included). Expected columns
   * (Chinese Alipay daily bill format):
   *   商户订单号, 支付宝交易号, 商户实收 (元), 商户手续费 (元), ...
   *
   * For demo purposes we accept either Chinese or English headers and key off
   * column index, not name, to stay tolerant of minor format drift.
   */
  parseAlipayCsv(csv: string): BillRow[] {
    return this.parseCsv(csv, { orderNoCol: 0, tradeNoCol: 1, amountCol: 2, feeCol: 3 });
  }

  /**
   * Parse a PayPal-style bill (PayPal transaction CSV). Columns vary widely
   * by report type, but for our minimal needs we use:
   *   transaction_id, invoice_id, gross, fee
   */
  parsePayPalCsv(csv: string): BillRow[] {
    return this.parseCsv(csv, { tradeNoCol: 0, orderNoCol: 1, amountCol: 2, feeCol: 3 });
  }

  private parseCsv(
    csv: string,
    cols: { orderNoCol: number; tradeNoCol: number; amountCol: number; feeCol: number },
  ): BillRow[] {
    // `csv-parse` handles quoted fields, escaped quotes (""), and CRLF/LF for us.
    // We still key off column index (not header name) for tolerance to format drift.
    const rows = parseCsvSync(csv, {
      skip_empty_lines: true,
      trim: true,
      relax_column_count: true,
      from_line: 2, // skip header
    }) as string[][];

    const out: BillRow[] = [];
    for (const parts of rows) {
      const orderNo = (parts[cols.orderNoCol] || '').trim();
      const tradeNo = (parts[cols.tradeNoCol] || '').trim();
      const amount = yuanStringToCents(parts[cols.amountCol]);
      const fee = yuanStringToCents(parts[cols.feeCol]);
      if (!orderNo && !tradeNo) continue;
      out.push({ orderNo, upstreamTradeNo: tradeNo, amountCents: amount, feeCents: fee });
    }
    return out;
  }

  /**
   * Compare a list of bill rows from one PSP against local orders for the
   * given date and persist a record per row. Returns aggregate counters.
   *
   * Matching key: `BillRow.orderNo` ↔ `PaymentOrder.orderNo`.
   * Local orders not present in the bill produce a `MissingUpstream` row.
   */
  async reconcile(provider: string, billDate: string, rows: BillRow[]) {
    const start = new Date(`${billDate}T00:00:00`);
    const end = new Date(`${billDate}T23:59:59.999`);

    const localOrders = await this.orderRepository.find({
      where: {
        payMethod: provider,
        status: OrderStatus.Paid,
        payAt: Between(start, end),
      },
    });
    const localByOrderNo = new Map(localOrders.map(o => [o.orderNo, o]));
    const seenLocal = new Set<string>();

    let matched = 0, mismatched = 0, missingLocal = 0, missingUpstream = 0;

    for (const row of rows) {
      const local = localByOrderNo.get(row.orderNo);
      let status: ReconStatus;
      let note: string | undefined;

      if (!local) {
        status = ReconStatus.MissingLocal;
        note = 'Upstream bill row has no matching local order';
        missingLocal++;
      } else {
        seenLocal.add(local.orderNo);
        if (Math.abs(row.amountCents - local.amount) <= 1) {
          status = ReconStatus.Matched;
          matched++;
        } else {
          status = ReconStatus.AmountMismatch;
          note = `local=${toYuan(local.amount)} upstream=${toYuan(row.amountCents)}`;
          mismatched++;
        }
      }

      await this.reconRepository.save(
        this.reconRepository.create({
          provider,
          billDate,
          orderNo: row.orderNo,
          upstreamTradeNo: row.upstreamTradeNo,
          upstreamAmount: row.amountCents,
          localAmount: local?.amount ?? 0,
          upstreamFee: row.feeCents,
          status,
          note,
        }),
      );
    }

    // Orders paid locally but absent from the upstream bill.
    for (const local of localOrders) {
      if (seenLocal.has(local.orderNo)) continue;
      missingUpstream++;
      await this.reconRepository.save(
        this.reconRepository.create({
          provider,
          billDate,
          orderNo: local.orderNo,
          upstreamTradeNo: local.thirdPartyTradeNo,
          upstreamAmount: 0,
          localAmount: local.amount,
          upstreamFee: 0,
          status: ReconStatus.MissingUpstream,
          note: 'Local order has no matching upstream bill row',
        }),
      );
    }

    const summary = { provider, billDate, matched, mismatched, missingLocal, missingUpstream };
    this.logger.log(`Reconciliation done: ${JSON.stringify(summary)}`);
    return summary;
  }

  /**
   * Fetch reconciliation records for an admin UI. Filters are all optional.
   */
  async list(opts: {
    provider?: string;
    billDate?: string;
    status?: ReconStatus;
    page?: number;
    pageSize?: number;
  } = {}) {
    const page = Math.max(1, Number(opts.page) || 1);
    const pageSize = Math.min(200, Math.max(1, Number(opts.pageSize) || 50));

    const qb = this.reconRepository
      .createQueryBuilder('r')
      .orderBy('r.createdAt', 'DESC')
      .skip((page - 1) * pageSize)
      .take(pageSize);
    if (opts.provider) qb.andWhere('r.provider = :p', { p: opts.provider });
    if (opts.billDate) qb.andWhere('r.billDate = :d', { d: opts.billDate });
    if (opts.status) qb.andWhere('r.status = :s', { s: opts.status });

    const [items, total] = await qb.getManyAndCount();
    return {
      items: items.map(r => ({
        id: r.id,
        provider: r.provider,
        billDate: r.billDate,
        orderNo: r.orderNo,
        upstreamTradeNo: r.upstreamTradeNo,
        upstreamAmount: toYuan(r.upstreamAmount),
        localAmount: toYuan(r.localAmount),
        upstreamFee: toYuan(r.upstreamFee),
        status: r.status,
        note: r.note,
        createdAt: r.createdAt,
      })),
      total,
      page,
      pageSize,
    };
  }
}
