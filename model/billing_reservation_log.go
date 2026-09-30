package model

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func FindBillingReservationLog(requestID string, userID int) (*Log, error) {
	if requestID == "" {
		return nil, nil
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return nil, errors.New("BILLING_JOURNAL_STORE_UNSUPPORTED")
	}
	if LOG_DB == nil {
		return nil, errors.New("billing journal database unavailable")
	}
	var log Log
	err := LOG_DB.Where("request_id = ? AND user_id = ? AND (content LIKE ? OR content = ?)", requestID, userID, "BILLING_RESERVATION_%", "UNSUPPORTED_DFLOP_CACHE_TTL_1H").First(&log).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &log, err
}

// CreateBillingReservationLog serializes duplicate request identities on the
// primary user row, also when the usage log database is configured separately.
func CreateBillingReservationLog(log *Log) error {
	if LOG_DB == nil || common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return errors.New("BILLING_JOURNAL_STORE_UNSUPPORTED")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id").First(&user, "id = ?", log.UserId).Error; err != nil {
			return err
		}
		if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
			if err := tx.Model(&User{}).Where("id = ?", log.UserId).UpdateColumn("quota", gorm.Expr("quota")).Error; err != nil {
				return err
			}
		}
		logs := LOG_DB
		if LOG_DB == DB {
			logs = tx
		}
		var count int64
		if err := logs.Model(&Log{}).Where("request_id = ? AND user_id = ? AND (content LIKE ? OR content = ?)", log.RequestId, log.UserId, "BILLING_RESERVATION_%", "UNSUPPORTED_DFLOP_CACHE_TTL_1H").Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return errors.New("BILLING_RESERVATION_REPLAY_REQUIRES_MANUAL_RESOLUTION")
		}
		return logs.Create(log).Error
	})
}

func TransitionBillingReservationLog(id int, from, to string, other *LogOther) error {
	updates := map[string]any{"content": "BILLING_RESERVATION_" + to, "other": other.JSONString()}
	if to == "QUARANTINED" {
		updates["content"] = "UNSUPPORTED_DFLOP_CACHE_TTL_1H"
		updates["type"] = LogTypeConsume
	}
	result := LOG_DB.Model(&Log{}).Where("id = ? AND content = ?", id, "BILLING_RESERVATION_"+from).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 && from == to {
		var count int64
		if err := LOG_DB.Model(&Log{}).Where("id = ? AND content = ?", id, "BILLING_RESERVATION_"+from).Count(&count).Error; err != nil {
			return err
		}
		if count == 1 {
			return nil
		}
	}
	if result.RowsAffected != 1 {
		return errors.New("billing reservation state conflict")
	}
	return nil
}
