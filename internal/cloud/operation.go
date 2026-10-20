package cloud

import (
	"fmt"
	"time"
)

// NewOperation 构造一条 pending 操作。ID 由 kind 与厂商请求 ID 确定性拼接，
// 不引入随机源（同一次云操作在重试/落库间保持同一 ID）。
func NewOperation(kind OperationKind, vendorRequestID string, now time.Time) Operation {
	return Operation{
		ID:              fmt.Sprintf("%s-%s", kind, vendorRequestID),
		Kind:            kind,
		VendorRequestID: vendorRequestID,
		Status:          OperationPending,
		UpdatedAt:       now,
	}
}

// IsTerminal 判断是否终态。
func (o Operation) IsTerminal() bool {
	return o.Status == OperationSucceeded || o.Status == OperationFailed
}

// WithStatus 推进状态。合法迁移：pending → running、running → succeeded|failed。
// 终态拒绝再驱动；其余组合为非法迁移。错误为厂商无关的编排层错误。
func (o Operation) WithStatus(next OperationStatus, now time.Time) (Operation, error) {
	if o.IsTerminal() {
		return o, fmt.Errorf("cloud: operation %s in terminal state %s cannot transition to %s", o.ID, o.Status, next)
	}
	if !allowedTransition(o.Status, next) {
		return o, fmt.Errorf("cloud: operation %s cannot transition from %s to %s", o.ID, o.Status, next)
	}
	advanced := o
	advanced.Status = next
	advanced.UpdatedAt = now
	return advanced, nil
}

// allowedTransition 枚举合法迁移边。
func allowedTransition(from, to OperationStatus) bool {
	switch from {
	case OperationPending:
		return to == OperationRunning
	case OperationRunning:
		return to == OperationSucceeded || to == OperationFailed
	default:
		return false
	}
}

// MarkSucceeded 推进到 succeeded 并记录资源 ID（create 场景）。
// 厂商错误文本在此保留原样；本方法只做编排层状态推进。
func (o Operation) MarkSucceeded(nativeID string, now time.Time) (Operation, error) {
	done, err := o.WithStatus(OperationSucceeded, now)
	if err != nil {
		return o, err
	}
	done.NativeID = nativeID
	return done, nil
}

// MarkFailed 推进到 failed 并保留厂商错误原文（不吞错、不翻译）。
func (o Operation) MarkFailed(vendorError string, now time.Time) (Operation, error) {
	failed, err := o.WithStatus(OperationFailed, now)
	if err != nil {
		return o, err
	}
	failed.Error = vendorError
	return failed, nil
}
