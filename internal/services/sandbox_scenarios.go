package services

import (
	"fmt"
	"math"
	"strings"
	"time"

	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/money"
)

type SandboxScenarioInput struct {
	OrderNo      string
	Scenario     string
	DelaySeconds int
	Amount       float64
}

type SandboxScenarioService struct {
	store    *db.Store
	payments *PaymentService
}

func NewSandboxScenarioService(store *db.Store, payments *PaymentService) *SandboxScenarioService {
	return &SandboxScenarioService{store: store, payments: payments}
}

func (s *SandboxScenarioService) Create(input SandboxScenarioInput) (*db.SandboxTask, error) {
	input.OrderNo = strings.TrimSpace(input.OrderNo)
	if input.OrderNo == "" {
		return nil, httperr.BadRequest("请填写订单号")
	}
	if input.DelaySeconds < 0 || input.DelaySeconds > 3600 {
		return nil, httperr.BadRequest("延迟时间必须为 0–3600 秒")
	}
	switch input.Scenario {
	case "paid", "failed", "expired", "delayed_paid", "refund_failed", "refund_pending":
	default:
		return nil, httperr.BadRequest("无效沙箱场景")
	}
	if input.Scenario == "delayed_paid" || input.Scenario == "refund_pending" {
		if input.DelaySeconds == 0 {
			input.DelaySeconds = 30
		}
	} else {
		input.DelaySeconds = 0
	}
	task := &db.SandboxTask{OrderNo: input.OrderNo, Scenario: input.Scenario, RunAt: time.Now().Add(time.Duration(input.DelaySeconds) * time.Second)}
	err := s.store.Do(func(tx *db.TxScope) error {
		order, err := tx.Orders.FindByOrderNo(task.OrderNo, false)
		if err != nil {
			return err
		}
		if order == nil {
			return httperr.NotFound("订单不存在")
		}
		if order.PayMethod != "native" {
			return httperr.Conflict("场景模拟只支持自有通道订单")
		}
		active, err := tx.SandboxTasks().ActiveForOrder(task.OrderNo)
		if err != nil {
			return err
		}
		if active {
			return httperr.Conflict("此订单已有待执行场景，请先完成或取消")
		}
		refund := strings.HasPrefix(task.Scenario, "refund_")
		if refund {
			if order.Status != db.StatusPaid {
				return httperr.Conflict("退款场景需要已支付订单")
			}
			remaining := order.Amount - order.RefundedAmount
			if input.Amount == 0 {
				task.Amount = remaining
			} else if math.IsNaN(input.Amount) || math.IsInf(input.Amount, 0) || input.Amount < 0.01 || input.Amount >= float64(math.MaxInt64)/100 {
				return httperr.BadRequest("退款金额无效")
			} else {
				task.Amount = money.YuanToCents(input.Amount)
			}
			if task.Amount <= 0 || task.Amount > remaining {
				return httperr.BadRequest("退款金额超过可退余额")
			}
		} else if order.Status != db.StatusPending || !order.ExpireAt.After(time.Now()) {
			return httperr.Conflict("支付场景需要未到期的待支付订单")
		}
		if task.Scenario == "delayed_paid" && !task.RunAt.Before(order.ExpireAt) {
			return httperr.BadRequest("延迟到账时间不能超过订单支付期限")
		}
		if err := tx.SandboxTasks().Insert(task); err != nil {
			return err
		}
		if refund {
			requestID := sandboxRefundRequestID(task.ID)
			status := db.RefundFailed
			if task.Scenario == "refund_pending" {
				status = db.RefundPending
			}
			op := &db.RefundOperation{ID: refundOperationID(task.OrderNo, requestID), RequestID: requestID, Reason: "沙箱场景模拟", OrderNo: task.OrderNo, Amount: task.Amount, Status: status}
			if err := tx.Refunds.Insert(op); err != nil {
				return err
			}
			if task.Scenario == "refund_pending" {
				return tx.Orders.Update(task.OrderNo, map[string]any{"status": db.StatusRefunding})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if input.DelaySeconds == 0 {
		if err := s.RunDue(); err != nil {
			return nil, err
		}
		return s.store.SandboxTasks().Find(task.ID)
	}
	return task, nil
}

func sandboxRefundRequestID(id int64) string { return fmt.Sprintf("sandbox_task_%d", id) }

func (s *SandboxScenarioService) Cancel(id int64) (*db.SandboxTask, error) {
	err := s.store.Do(func(tx *db.TxScope) error {
		task, err := tx.SandboxTasks().Find(id)
		if err != nil {
			return err
		}
		if task == nil {
			return httperr.NotFound("场景任务不存在")
		}
		if task.Status == "cancelled" {
			return nil
		}
		if task.Status != "pending" && !(task.Status == "failed" && task.Scenario == "refund_pending") {
			return httperr.Conflict("此任务已执行或正在执行")
		}
		if task.Scenario == "refund_pending" {
			opID := refundOperationID(task.OrderNo, sandboxRefundRequestID(task.ID))
			op, err := tx.Refunds.FindByID(opID)
			if err != nil {
				return err
			}
			order, err := tx.Orders.FindByOrderNo(task.OrderNo, false)
			if err != nil {
				return err
			}
			if op != nil && op.Status == db.RefundPending && order != nil && order.Status == db.StatusRefunding {
				if err := tx.Refunds.Update(opID, db.RefundFailed, nil); err != nil {
					return err
				}
				if err := tx.Orders.Update(task.OrderNo, map[string]any{"status": db.StatusPaid}); err != nil {
					return err
				}
			}
		}
		return tx.SandboxTasks().Finish(id, "cancelled", "")
	})
	if err != nil {
		return nil, err
	}
	return s.store.SandboxTasks().Find(id)
}

func (s *SandboxScenarioService) RunDue() error {
	tasks, err := s.store.SandboxTasks().Due()
	if err != nil {
		return err
	}
	for _, task := range tasks {
		claimed, err := s.store.SandboxTasks().Claim(task.ID)
		if err != nil {
			return err
		}
		if !claimed {
			continue
		}
		status, message := "completed", ""
		if err := s.apply(task); err != nil {
			status, message = "failed", err.Error()
		}
		if err := s.store.SandboxTasks().Finish(task.ID, status, message); err != nil {
			return err
		}
	}
	return nil
}

func (s *SandboxScenarioService) apply(task *db.SandboxTask) error {
	order, err := s.store.Orders.FindByOrderNo(task.OrderNo, false)
	if err != nil {
		return err
	}
	if order == nil {
		return httperr.NotFound("订单已删除")
	}
	if order.PayMethod != "native" {
		return httperr.Conflict("订单已切换外部通道，停止场景模拟")
	}
	switch task.Scenario {
	case "paid", "delayed_paid":
		return s.store.Do(func(tx *db.TxScope) error {
			current, err := tx.Orders.FindByOrderNo(task.OrderNo, false)
			if err != nil {
				return err
			}
			if current == nil {
				return httperr.NotFound("订单已删除")
			}
			if current.PayMethod != "native" {
				return httperr.Conflict("订单已切换外部通道")
			}
			if current.Status == db.StatusPaid || current.Status == db.StatusRefunding || current.Status == db.StatusRefunded {
				return nil
			}
			if current.Status != db.StatusPending || !current.ExpireAt.After(time.Now()) {
				return httperr.Conflict("订单已结束或到期")
			}
			tradeNo := fmt.Sprintf("SP_SANDBOX_%d", task.ID)
			paidAt := time.Now()
			fees := s.payments.fees.Calculate("native", current.Amount)
			if err := tx.Orders.Update(task.OrderNo, map[string]any{"status": db.StatusPaid, "payAt": paidAt, "thirdPartyTradeNo": tradeNo, "fee": fees.Fee, "channelCost": fees.ChannelCost, "settleAmount": fees.SettleAmount}); err != nil {
				return err
			}
			current.Status, current.PayAt, current.ThirdPartyTradeNo = db.StatusPaid, &paidAt, &tradeNo
			current.Fee, current.ChannelCost, current.SettleAmount = fees.Fee, fees.ChannelCost, fees.SettleAmount
			return s.payments.enqueueStateNotification(tx, current, "paid", &tradeNo)
		})
	case "failed":
		return s.store.Do(func(tx *db.TxScope) error {
			current, err := tx.Orders.FindByOrderNo(task.OrderNo, false)
			if err != nil {
				return err
			}
			if current == nil {
				return httperr.NotFound("订单已删除")
			}
			if current.PayMethod != "native" {
				return httperr.Conflict("订单已切换外部通道")
			}
			if current.Status == db.StatusFailed {
				return nil
			}
			if current.Status != db.StatusPending {
				return httperr.Conflict("订单状态已改变")
			}
			return tx.Orders.Update(task.OrderNo, map[string]any{"status": db.StatusFailed})
		})
	case "expired":
		return s.store.Do(func(tx *db.TxScope) error {
			current, err := tx.Orders.FindByOrderNo(task.OrderNo, false)
			if err != nil {
				return err
			}
			if current == nil {
				return httperr.NotFound("订单已删除")
			}
			if current.PayMethod != "native" {
				return httperr.Conflict("订单已切换外部通道")
			}
			if current.Status == db.StatusExpired {
				return nil
			}
			if current.Status != db.StatusPending {
				return httperr.Conflict("订单状态已改变")
			}
			return tx.Orders.Update(task.OrderNo, map[string]any{"status": db.StatusExpired, "expireAt": time.Now().Add(-time.Second)})
		})
	case "refund_failed":
		return nil
	case "refund_pending":
		requestID := sandboxRefundRequestID(task.ID)
		opID := refundOperationID(task.OrderNo, requestID)
		tradeNo := fmt.Sprintf("SP_SANDBOX_REFUND_%d", task.ID)
		_, err = s.payments.completeRefund(task.OrderNo, task.Amount, tradeNo, nil, opID)
		return err
	}
	return httperr.BadRequest("无效沙箱场景")
}
