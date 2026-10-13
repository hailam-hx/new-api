package model

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	TaskIdempotencyClaimed       = "claimed"
	TaskIdempotencyIndeterminate = "indeterminate"
	TaskIdempotencyDurable       = "durable"
	TaskIdempotencyRetryWindow   = 48 * time.Hour
)

var (
	ErrTaskIdempotencyConflict  = errors.New("idempotency key conflicts with this request")
	ErrTaskIdempotencyClaimLost = errors.New("idempotency claim is unavailable")
)

// TaskIdempotency contains only bounded identities and a reference to the exact
// durable task row. It never stores client keys, credentials or request bodies.
type TaskIdempotency struct {
	ID               int64  `gorm:"primaryKey"`
	UserID           int    `gorm:"not null"`
	TokenID          int    `gorm:"not null;uniqueIndex:idx_task_idempotency_identity,priority:1"`
	OperationHash    string `gorm:"type:varchar(64);not null;uniqueIndex:idx_task_idempotency_identity,priority:2"`
	KeyHash          string `gorm:"type:varchar(64);not null;uniqueIndex:idx_task_idempotency_identity,priority:3"`
	Fingerprint      string `gorm:"type:varchar(64);not null"`
	Nonce            string `gorm:"type:varchar(64);not null"`
	State            string `gorm:"type:varchar(20);not null;index:idx_task_idempotency_expiry,priority:1"`
	ResourceRecordID int64
	CreatedAt        int64
	UpdatedAt        int64
	ExpiresAt        int64 `gorm:"index:idx_task_idempotency_expiry,priority:2"`
}

// AcquireTaskIdempotency atomically inserts an identity, then verifies its nonce.
// MySQL clientFoundRows can report an affected row for a conflicting no-op;
// RowsAffected must never decide ownership of a paid-work claim.
func AcquireTaskIdempotency(ctx context.Context, userID, tokenID int, operationHash, keyHash, fingerprint string, now int64) (*TaskIdempotency, bool, error) {
	if userID <= 0 || tokenID <= 0 {
		return nil, false, ErrTaskIdempotencyConflict
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, false, err
	}
	timeoutMinutes := int64(max(constant.TaskTimeoutMinutes, 1440))
	if now < 0 || timeoutMinutes > (math.MaxInt64-now-int64(TaskIdempotencyRetryWindow/time.Second))/60 {
		return nil, false, errors.New("invalid task idempotency retention")
	}
	retention := timeoutMinutes*60 + int64(TaskIdempotencyRetryWindow/time.Second)
	candidate := TaskIdempotency{UserID: userID, TokenID: tokenID, OperationHash: operationHash, KeyHash: keyHash, Fingerprint: fingerprint, Nonce: hex.EncodeToString(random[:]), State: TaskIdempotencyClaimed, CreatedAt: now, UpdatedAt: now, ExpiresAt: now + retention}
	for range 3 {
		if err := DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error; err != nil {
			return nil, false, err
		}
		var current TaskIdempotency
		if err := DB.WithContext(ctx).Where("token_id = ? AND operation_hash = ? AND key_hash = ?", tokenID, operationHash, keyHash).First(&current).Error; err != nil {
			return nil, false, err
		}
		if current.Nonce == candidate.Nonce {
			return &current, true, nil
		}
		if current.UserID != userID {
			return nil, false, ErrTaskIdempotencyConflict
		}
		if current.State == TaskIdempotencyDurable && current.ExpiresAt <= now && current.ResourceRecordID > 0 {
			var task Task
			if err := DB.WithContext(ctx).First(&task, current.ResourceRecordID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, false, err
			}
			if task.ID > 0 && (task.Status == TaskStatusSuccess || task.Status == TaskStatusFailure) && task.FinishTime > 0 && task.FinishTime <= now-int64(TaskIdempotencyRetryWindow/time.Second) {
				deleted := DB.WithContext(ctx).Where("id = ? AND nonce = ? AND state = ? AND resource_record_id = ?", current.ID, current.Nonce, TaskIdempotencyDurable, task.ID).Delete(&TaskIdempotency{})
				if deleted.Error != nil {
					return nil, false, deleted.Error
				}
				candidate.ID = 0
				continue
			}
		}
		if current.Fingerprint != fingerprint {
			return nil, false, ErrTaskIdempotencyConflict
		}
		return &current, false, nil
	}
	return nil, false, ErrTaskIdempotencyClaimLost
}

// BeginTaskIdempotencyOutbound must commit before any call that may create paid
// work. A crashed or uncertain attempt remains indeterminate indefinitely.
func BeginTaskIdempotencyOutbound(ctx context.Context, id int64, nonce string) error {
	result := DB.WithContext(ctx).Model(&TaskIdempotency{}).Where("id = ? AND nonce = ? AND state = ? AND expires_at > ?", id, nonce, TaskIdempotencyClaimed, time.Now().Unix()).Updates(map[string]any{"state": TaskIdempotencyIndeterminate, "updated_at": time.Now().Unix()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrTaskIdempotencyClaimLost
	}
	return nil
}

func ReleaseTaskIdempotencyClaim(ctx context.Context, id int64, nonce string) error {
	return DB.WithContext(ctx).Where("id = ? AND nonce = ? AND state = ?", id, nonce, TaskIdempotencyClaimed).Delete(&TaskIdempotency{}).Error
}

// InsertTaskWithIdempotency makes task persistence and replay eligibility one
// main-database transaction. A failed binding rolls back the task as well.
func InsertTaskWithIdempotency(ctx context.Context, task *Task, id int64, nonce string, omitColumns ...string) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var claim TaskIdempotency
		if err := lockForUpdate(tx).Where("id = ? AND nonce = ? AND state = ?", id, nonce, TaskIdempotencyIndeterminate).First(&claim).Error; err != nil {
			return err
		}
		if claim.UserID != task.UserId || claim.TokenID != task.TokenId {
			return ErrTaskIdempotencyClaimLost
		}
		if err := tx.Omit(omitColumns...).Create(task).Error; err != nil {
			return err
		}
		result := tx.Model(&TaskIdempotency{}).Where("id = ? AND nonce = ? AND state = ?", id, nonce, TaskIdempotencyIndeterminate).Updates(map[string]any{"state": TaskIdempotencyDurable, "resource_record_id": task.ID, "updated_at": time.Now().Unix()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrTaskIdempotencyClaimLost
		}
		return nil
	})
}

// CleanupTaskIdempotency expires terminal task bindings only. A claimed row
// may survive a crash after pre-consume but before outbound marking, so claimed,
// missing, active and indeterminate work requires reconciliation and is retained.
func CleanupTaskIdempotency(ctx context.Context, now int64) error {
	terminalTasks := DB.WithContext(ctx).Model(&Task{}).Select("id").Where("status IN ? AND finish_time > 0 AND finish_time <= ?", []TaskStatus{TaskStatusSuccess, TaskStatusFailure}, now-int64(TaskIdempotencyRetryWindow/time.Second))
	return DB.WithContext(ctx).Where("state = ? AND expires_at <= ? AND resource_record_id IN (?)", TaskIdempotencyDurable, now, terminalTasks).Delete(&TaskIdempotency{}).Error
}
