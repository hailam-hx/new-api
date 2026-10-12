package model

import (
	"context"
	"encoding/hex"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

func GetTaskForToken(userID, tokenID int, taskID string) (*Task, bool, error) {
	return GetTaskForTokenWithContext(context.Background(), userID, tokenID, taskID)
}

// GetTaskForTokenWithContext never falls back to account ownership. Duplicate
// historical public IDs are indistinguishable from absent or foreign tasks.
func GetTaskForTokenWithContext(ctx context.Context, userID, tokenID int, taskID string) (*Task, bool, error) {
	if userID <= 0 || tokenID <= 0 || taskID == "" {
		return nil, false, nil
	}
	var tasks []*Task
	if err := DB.WithContext(ctx).Where("user_id = ? AND token_id = ? AND task_id = ?", userID, tokenID, taskID).Order("id").Limit(2).Find(&tasks).Error; err != nil {
		return nil, false, err
	}
	if len(tasks) != 1 || !tasks[0].HasCanonicalOwnership() || !tasks[0].ResultRetrievable() {
		return nil, false, nil
	}
	return tasks[0], true, nil
}

func GetTaskForTokenObservation(ctx context.Context, userID, tokenID int, platform constant.TaskPlatform, taskID string) (*Task, bool, error) {
	task, exists, err := GetTaskForTokenWithContext(ctx, userID, tokenID, taskID)
	if err != nil || !exists || task.Platform != platform {
		return nil, false, err
	}
	return task, true, nil
}

// GetTasksForTokenPlatforms performs one bounded ID-set lookup for native
// queries. Ambiguous IDs are removed instead of selecting an arbitrary row.
func GetTasksForTokenPlatforms(userID, tokenID int, platforms []constant.TaskPlatform, taskIDs []string) ([]*Task, error) {
	if userID <= 0 || tokenID <= 0 || len(platforms) == 0 || len(taskIDs) == 0 {
		return []*Task{}, nil
	}
	var tasks []*Task
	if err := DB.Where("user_id = ? AND token_id = ? AND task_id IN ?", userID, tokenID, taskIDs).Find(&tasks).Error; err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(tasks))
	for _, task := range tasks {
		counts[task.TaskID]++
	}
	owned := make([]*Task, 0, len(tasks))
	for _, task := range tasks {
		if counts[task.TaskID] != 1 || !task.HasCanonicalOwnership() || !task.ResultRetrievable() {
			continue
		}
		if slices.Contains(platforms, task.Platform) {
			owned = append(owned, task)
		}
	}
	return owned, nil
}

// ListTasksForToken uses the composite owner index and excludes ambiguous IDs
// in SQL. The cursor advances over at most one bounded page, including rows
// whose private provenance contradicts their materialized owner.
func ListTasksForToken(ctx context.Context, userID, tokenID int, beforeID int64, limit int) ([]*Task, int64, error) {
	owned := make([]*Task, 0)
	if userID <= 0 || tokenID <= 0 {
		return owned, 0, nil
	}
	limit = min(max(limit, 1), 100)
	uniqueIDs := DB.Model(&Task{}).Select("task_id").Where("user_id = ? AND token_id = ? AND task_id <> ''", userID, tokenID).Group("task_id").Having("COUNT(*) = 1")
	query := DB.WithContext(ctx).Model(&Task{}).Select("id", "task_id", "token_id", "user_id", "request_id", "platform", "status", "progress", "created_at", "submit_time", "start_time", "finish_time", "properties", "private_data").Where("user_id = ? AND token_id = ? AND task_id IN (?)", userID, tokenID, uniqueIDs)
	if beforeID > 0 {
		query = query.Where("id < ?", beforeID)
	}
	var tasks []*Task
	if err := query.Order("id DESC").Limit(limit + 1).Find(&tasks).Error; err != nil {
		return nil, 0, err
	}
	var nextID int64
	if len(tasks) > limit {
		nextID = tasks[limit-1].ID
		tasks = tasks[:limit]
	}
	for _, task := range tasks {
		if task.HasCanonicalOwnership() && task.ResultRetrievable() {
			owned = append(owned, task)
		}
	}
	return owned, nextID, nil
}

func GetMidjourneyForToken(userID, tokenID int, taskID string) (*Midjourney, error) {
	if userID <= 0 || tokenID <= 0 || taskID == "" {
		return nil, nil
	}
	var tasks []*Midjourney
	if err := DB.Where("user_id = ? AND owner_token_id = ? AND mj_id = ?", userID, tokenID, taskID).Order("id").Limit(2).Find(&tasks).Error; err != nil {
		return nil, err
	}
	if len(tasks) != 1 {
		return nil, nil
	}
	return tasks[0], nil
}

func GetMidjourneysForToken(userID, tokenID int, taskIDs []string) ([]*Midjourney, error) {
	if userID <= 0 || tokenID <= 0 || len(taskIDs) == 0 {
		return []*Midjourney{}, nil
	}
	var tasks []*Midjourney
	if err := DB.Where("user_id = ? AND owner_token_id = ? AND mj_id IN ?", userID, tokenID, taskIDs).Find(&tasks).Error; err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(tasks))
	for _, task := range tasks {
		counts[task.MjId]++
	}
	owned := make([]*Midjourney, 0, len(tasks))
	for _, task := range tasks {
		if counts[task.MjId] == 1 {
			owned = append(owned, task)
		}
	}
	return owned, nil
}

func GetUniqueMidjourneyForImage(taskID string) (*Midjourney, error) {
	if taskID == "" {
		return nil, nil
	}
	var tasks []*Midjourney
	if err := DB.Where("mj_id = ?", taskID).Order("id").Limit(2).Find(&tasks).Error; err != nil {
		return nil, err
	}
	if len(tasks) != 1 {
		return nil, nil
	}
	return tasks[0], nil
}

// MigrateTaskOwnership materializes only deterministic historical provenance.
// Bounded primary-key batches avoid loading the full task history. JSON is
// decoded in Go so the same migration works on every primary database dialect.
// Existing canonical assignments and legacy billing fields are never changed.
func MigrateTaskOwnership(db *gorm.DB) error {
	var after int64
	for {
		var rows []struct {
			ID          int64
			UserId      int
			TokenId     int
			RequestId   string
			PrivateData string
		}
		if err := db.Model(&Task{}).Select("id", "user_id", "token_id", "request_id", "private_data").Where("id > ? AND ((token_id = 0 OR token_id IS NULL) OR (request_id = '' OR request_id IS NULL))", after).Order("id").Limit(200).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		private := make([]TaskPrivateData, len(rows))
		valid := make([]bool, len(rows))
		ownerIDs := make([]int, 0, len(rows))
		for i, row := range rows {
			if row.PrivateData == "" || common.UnmarshalJsonStr(row.PrivateData, &private[i]) != nil {
				continue
			}
			valid[i] = true
			if private[i].TokenId > 0 {
				ownerIDs = append(ownerIDs, private[i].TokenId)
			}
		}
		owners, err := taskOwnershipTokenUsers(db, ownerIDs)
		if err != nil {
			return err
		}
		for i, row := range rows {
			if !valid[i] {
				continue
			}
			provenance := private[i]
			if provenance.TokenId <= 0 || owners[provenance.TokenId] != row.UserId || row.UserId <= 0 {
				continue
			}
			if row.TokenId != 0 && row.TokenId != provenance.TokenId {
				continue
			}
			if provenance.Execution != nil && provenance.Execution.RuntimeVerificationItemID > 0 {
				continue
			}
			if row.TokenId == 0 {
				if err := db.Model(&Task{}).Where("id = ? AND (token_id = 0 OR token_id IS NULL)", row.ID).Update("token_id", provenance.TokenId).Error; err != nil {
					return err
				}
			}
			if row.RequestId != "" || provenance.Execution == nil {
				continue
			}
			requestID := provenance.Execution.CanonicalRequestID
			if requestID == "" {
				requestID = provenance.Execution.RequestID
				// Older DFLOP submissions replaced the HTTP request ID with a
				// SHA-256 billing-operation digest. It is not HTTP correlation.
				if len(requestID) == 64 {
					if _, err := hex.DecodeString(requestID); err == nil {
						requestID = ""
					}
				}
			}
			if requestID == "" || len(requestID) > 64 || strings.TrimSpace(requestID) != requestID || strings.ContainsAny(requestID, "\r\n\x00") {
				continue
			}
			if err := db.Model(&Task{}).Where("id = ? AND token_id = ? AND (request_id = '' OR request_id IS NULL)", row.ID, provenance.TokenId).Update("request_id", requestID).Error; err != nil {
				return err
			}
		}
		after = rows[len(rows)-1].ID
	}

	after = 0
	for {
		var rows []struct {
			Id           int64
			UserId       int
			TokenId      int
			OwnerTokenId int
		}
		if err := db.Model(&Midjourney{}).Select("id", "user_id", "token_id", "owner_token_id").Where("id > ? AND (owner_token_id = 0 OR owner_token_id IS NULL)", after).Order("id").Limit(200).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		ownerIDs := make([]int, 0, len(rows))
		for _, row := range rows {
			if row.TokenId > 0 && row.OwnerTokenId == 0 {
				ownerIDs = append(ownerIDs, row.TokenId)
			}
		}
		owners, err := taskOwnershipTokenUsers(db, ownerIDs)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row.OwnerTokenId != 0 || row.TokenId <= 0 || row.UserId <= 0 || owners[row.TokenId] != row.UserId {
				continue
			}
			if err := db.Model(&Midjourney{}).Where("id = ? AND (owner_token_id = 0 OR owner_token_id IS NULL)", row.Id).Update("owner_token_id", row.TokenId).Error; err != nil {
				return err
			}
		}
		after = rows[len(rows)-1].Id
	}
}

// taskOwnershipTokenUsers includes deleted keys because deletion revokes reads,
// but does not erase their historical identity. No raw key is selected.
func taskOwnershipTokenUsers(db *gorm.DB, ids []int) (map[int]int, error) {
	owners := make(map[int]int)
	if len(ids) == 0 {
		return owners, nil
	}
	var tokens []Token
	if err := db.Unscoped().Select("id", "user_id").Where("id IN ?", ids).Find(&tokens).Error; err != nil {
		return nil, err
	}
	for _, token := range tokens {
		owners[token.Id] = token.UserId
	}
	return owners, nil
}
