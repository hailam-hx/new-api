package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const taskIdempotencyJSONLimit = 16 << 20
const taskIdempotencyMultipartLimit = 128 << 20

// TaskIdempotencyFingerprint preserves exact numeric values and file contents.
// Multipart boundaries and JSON member order are transport details, not identity.
func TaskIdempotencyFingerprint(c *gin.Context) (string, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return "", err
	}
	reader, err := storage.NewReader()
	if err != nil {
		return "", err
	}
	defer reader.Close()
	mediaType, params, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil && c.GetHeader("Content-Type") != "" {
		return "", err
	}
	if mediaType == "" {
		mediaType = "application/json"
	}
	var body any
	switch mediaType {
	case "application/json":
		if storage.Size() > taskIdempotencyJSONLimit {
			return "", errors.New("idempotent JSON body exceeds 16 MiB")
		}
		data, err := io.ReadAll(io.LimitReader(reader, taskIdempotencyJSONLimit+1))
		if err != nil {
			return "", err
		}
		if len(data) > taskIdempotencyJSONLimit || !utf8.Valid(data) {
			return "", errors.New("idempotent request requires valid UTF-8 JSON")
		}
		// Bound recursive parser stack usage before validating JSON. Brackets
		// inside strings and escaped quotes do not introduce nesting.
		depth, quoted, escaped := 0, false, false
		for _, char := range data {
			if quoted {
				if escaped {
					escaped = false
				} else if char == '\\' {
					escaped = true
				} else if char == '"' {
					quoted = false
				}
				continue
			}
			switch char {
			case '"':
				quoted = true
			case '{', '[':
				depth++
				if depth > 32 {
					return "", errors.New("idempotent JSON exceeds structural limit")
				}
			case '}', ']':
				depth--
				if depth < 0 {
					return "", errors.New("invalid idempotent JSON framing")
				}
			}
		}
		if !gjson.ValidBytes(data) {
			return "", errors.New("idempotent request requires valid JSON")
		}
		nodes := 0
		body, err = canonicalTaskIdempotencyJSON(gjson.ParseBytes(data), 0, &nodes)
		if err != nil {
			return "", err
		}
	case "multipart/form-data":
		if params["boundary"] == "" || storage.Size() > taskIdempotencyMultipartLimit {
			return "", errors.New("unsupported idempotent multipart request")
		}
		parts := multipart.NewReader(reader, params["boundary"])
		fields := map[string][]any{}
		count := 0
		for {
			part, err := parts.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return "", err
			}
			count++
			name := part.FormName()
			if count > 128 || name == "" || len(name) > 256 || !utf8.ValidString(name) || len(part.FileName()) > 1024 || !utf8.ValidString(part.FileName()) || len(part.Header.Get("Content-Type")) > 1024 || !utf8.ValidString(part.Header.Get("Content-Type")) {
				_ = part.Close()
				return "", errors.New("unsupported idempotent multipart fields")
			}
			var value any
			if part.FileName() == "" {
				data, err := io.ReadAll(io.LimitReader(part, 1<<20+1))
				if err != nil {
					_ = part.Close()
					return "", err
				}
				if len(data) > 1<<20 || !utf8.Valid(data) {
					_ = part.Close()
					return "", errors.New("unsupported idempotent multipart field")
				}
				value = []any{"field", string(data)}
			} else {
				digest := sha256.New()
				size, err := io.Copy(digest, part)
				if err != nil {
					_ = part.Close()
					return "", err
				}
				value = []any{"file", part.FileName(), part.Header.Get("Content-Type"), size, fmt.Sprintf("%x", digest.Sum(nil))}
			}
			if err := part.Close(); err != nil {
				return "", err
			}
			fields[name] = append(fields[name], value)
		}
		body = fields
	default:
		return "", errors.New("idempotency supports JSON and multipart/form-data task submissions only")
	}
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		return "", errors.New("idempotent request requires valid query parameters")
	}
	if mediaType == "application/json" && strings.EqualFold(params["charset"], "utf-8") {
		delete(params, "charset")
	}
	if mediaType == "multipart/form-data" {
		delete(params, "boundary")
	}
	// These are the request headers exposed to task driver hooks; they can
	// affect provider work. All other opaque client headers remain excluded.
	encoded, err := common.Marshal([]any{strings.ToUpper(c.Request.Method), c.Request.URL.Path, query, mediaType, params, c.GetHeader("Accept"), body})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", digest), nil
}

// The tagged representation distinguishes numeric strings from JSON numbers
// while keeping 9007199254740992 and 9007199254740993 distinct without float64.
func canonicalTaskIdempotencyJSON(value gjson.Result, depth int, nodes *int) (any, error) {
	*nodes++
	if depth > 32 || *nodes > 65536 {
		return nil, errors.New("idempotent JSON exceeds structural limit")
	}
	switch value.Type {
	case gjson.JSON:
		if value.IsArray() {
			items := make([]any, 0)
			var parseErr error
			value.ForEach(func(_, child gjson.Result) bool {
				item, err := canonicalTaskIdempotencyJSON(child, depth+1, nodes)
				if err != nil {
					parseErr = err
					return false
				}
				items = append(items, item)
				return true
			})
			return []any{"array", items}, parseErr
		}
		members := map[string]any{}
		var parseErr error
		value.ForEach(func(key, child gjson.Result) bool {
			var name string
			if err := common.UnmarshalJsonStr(key.Raw, &name); err != nil {
				parseErr = err
				return false
			}
			if _, exists := members[name]; exists {
				parseErr = errors.New("duplicate JSON members are unsupported for idempotency")
				return false
			}
			item, err := canonicalTaskIdempotencyJSON(child, depth+1, nodes)
			if err != nil {
				parseErr = err
				return false
			}
			members[name] = item
			return true
		})
		return []any{"object", members}, parseErr
	case gjson.Number:
		if len(value.Raw) > 128 {
			return nil, errors.New("idempotent JSON number exceeds precision limit")
		}
		if _, exponent, found := strings.Cut(strings.ToLower(value.Raw), "e"); found {
			exp, err := strconv.Atoi(exponent)
			if err != nil || exp < -4096 || exp > 4096 {
				return nil, errors.New("idempotent JSON number exponent is unsupported")
			}
		}
		mantissa, exponent, _ := strings.Cut(strings.ToLower(value.Raw), "e")
		power := 0
		if exponent != "" {
			power, _ = strconv.Atoi(exponent)
		}
		negative := strings.HasPrefix(mantissa, "-")
		mantissa = strings.TrimPrefix(mantissa, "-")
		integer, fraction, _ := strings.Cut(mantissa, ".")
		coefficient := strings.TrimLeft(integer+fraction, "0")
		if coefficient == "" {
			return []any{"number", "0", 0}, nil
		}
		trimmed := strings.TrimRight(coefficient, "0")
		power += len(coefficient) - len(trimmed) - len(fraction)
		if negative {
			trimmed = "-" + trimmed
		}
		return []any{"number", trimmed, power}, nil
	case gjson.String:
		var decoded string
		if err := common.UnmarshalJsonStr(value.Raw, &decoded); err != nil {
			return nil, err
		}
		return []any{"string", decoded}, nil
	case gjson.True, gjson.False:
		return []any{"boolean", value.Bool()}, nil
	case gjson.Null:
		return []any{"null"}, nil
	default:
		return nil, errors.New("unsupported idempotent JSON value")
	}
}

// BeginTaskIdempotency runs before any billing or submit attempt. Replay grants
// no additional authority: normal authentication/routing must already pass.
func BeginTaskIdempotency(c *gin.Context, info *relaycommon.RelayInfo) (*model.TaskIdempotency, *model.Task, *dto.TaskError) {
	keys := c.Request.Header.Values("Idempotency-Key")
	if len(keys) == 0 {
		return nil, nil, nil
	}
	if len(keys) != 1 || len(keys[0]) == 0 || len(keys[0]) > 200 || strings.TrimSpace(keys[0]) != keys[0] {
		return nil, nil, TaskErrorWrapperLocal(errors.New("Idempotency-Key must contain 1 to 200 printable ASCII characters without surrounding whitespace"), "invalid_idempotency_key", http.StatusBadRequest)
	}
	for _, char := range keys[0] {
		if char < 32 || char > 126 {
			return nil, nil, TaskErrorWrapperLocal(errors.New("Idempotency-Key must contain printable ASCII characters"), "invalid_idempotency_key", http.StatusBadRequest)
		}
	}
	fingerprint, err := TaskIdempotencyFingerprint(c)
	if err != nil {
		return nil, nil, TaskErrorWrapperLocal(err, "idempotency_request_unsupported", http.StatusBadRequest)
	}
	operation := sha256.Sum256([]byte(strings.ToUpper(c.Request.Method) + "\x00" + c.Request.URL.Path))
	key := sha256.Sum256([]byte(keys[0]))
	record, owned, err := model.AcquireTaskIdempotency(c.Request.Context(), info.UserId, info.TokenId, fmt.Sprintf("%x", operation), fmt.Sprintf("%x", key), fingerprint, time.Now().Unix())
	if errors.Is(err, model.ErrTaskIdempotencyConflict) {
		logger.LogDebug(c, "task idempotency decision=conflict")
		return nil, nil, TaskErrorWrapperLocal(err, "idempotency_key_conflict", http.StatusConflict)
	}
	if err != nil {
		return nil, nil, TaskErrorWrapperLocal(errors.New("idempotency claim unavailable"), "idempotency_storage_unavailable", http.StatusServiceUnavailable)
	}
	if owned {
		info.TaskIdempotencyID = record.ID
		info.TaskIdempotencyNonce = record.Nonce
		logger.LogDebug(c, "task idempotency decision=new")
		return record, nil, nil
	}
	if record.State != model.TaskIdempotencyDurable {
		code, message := "idempotency_in_progress", "The request is still in progress; reuse the same Idempotency-Key without changing the request"
		if record.State == model.TaskIdempotencyIndeterminate {
			code, message = "idempotency_indeterminate", "The funding or upstream outcome is uncertain; do not submit this operation with a new key"
		}
		logger.LogDebug(c, "task idempotency decision=%s", record.State)
		return nil, nil, TaskErrorWrapperLocal(errors.New(message), code, http.StatusConflict)
	}
	var task model.Task
	if err := model.DB.WithContext(c.Request.Context()).Where("id = ? AND user_id = ? AND token_id = ?", record.ResourceRecordID, info.UserId, info.TokenId).First(&task).Error; err != nil || !task.HasCanonicalOwnership() || !task.ResultRetrievable() {
		return nil, nil, TaskErrorWrapperLocal(errors.New("The accepted task result cannot be replayed; do not resubmit with a new key"), "idempotency_replay_unavailable", http.StatusConflict)
	}
	logger.LogDebug(c, "task idempotency decision=replay")
	return record, &task, nil
}

func MarkTaskIdempotencyOutbound(c *gin.Context, info *relaycommon.RelayInfo) error {
	if info.TaskIdempotencyID == 0 {
		return nil
	}
	if err := model.BeginTaskIdempotencyOutbound(c.Request.Context(), info.TaskIdempotencyID, info.TaskIdempotencyNonce); err != nil {
		return err
	}
	info.TaskIdempotencyOutbound = true
	return nil
}

// ReleaseTaskIdempotencyPreflight releases only work that never entered a
// billing session. Refund schedules asynchronous work, so its return is not
// durable proof that funding was restored. Funded claims require reconciliation.
func ReleaseTaskIdempotencyPreflight(info *relaycommon.RelayInfo) {
	if info.TaskIdempotencyID == 0 || info.TaskIdempotencyOutbound || info.TaskIdempotencyFundingAttempted || info.Billing != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := model.ReleaseTaskIdempotencyClaim(ctx, info.TaskIdempotencyID, info.TaskIdempotencyNonce); err != nil {
		common.SysError("task idempotency preflight release failed")
	}
}

// StartTaskIdempotencyCleanup uses the existing master/hourly cleanup pattern.
// Abandoned claimed and indeterminate records require operator reconciliation.
func StartTaskIdempotencyCleanup() {
	if !common.IsMasterNode {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			if err := model.CleanupTaskIdempotency(ctx, time.Now().Unix()); err != nil {
				common.SysError("task idempotency cleanup failed")
			}
			cancel()
			<-ticker.C
		}
	}()
}

// Preserve the pre-operation-scoped DFLOP journal boundary for old clients.
func GuardLegacyDFLOPTaskIdempotency(info *relaycommon.RelayInfo, identities ...string) error {
	if info.TaskIdempotencyID == 0 {
		return nil
	}
	for _, identity := range slices.Compact(identities) {
		if model.LOG_DB == nil || common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
			return errors.New("legacy billing journal unavailable")
		}
		var records []model.Log
		if err := model.LOG_DB.Where("request_id = ? AND user_id = ? AND (token_id = ? OR token_id = 0) AND (content LIKE ? OR content = ?)", identity, info.UserId, info.TokenId, "BILLING_RESERVATION_%", "UNSUPPORTED_DFLOP_CACHE_TTL_1H").Limit(1).Find(&records).Error; err != nil {
			return err
		}
		if len(records) != 0 {
			return errors.New("BILLING_RESERVATION_REPLAY_REQUIRES_MANUAL_RESOLUTION")
		}
	}
	return nil
}

// TaskIdempotencyErrorMessage identifies host-owned, payload-free errors that
// protocol/native presenters must preserve instead of reclassifying as provider errors.
func TaskIdempotencyErrorMessage(code string) (string, bool) {
	switch code {
	case "invalid_idempotency_key":
		return "Idempotency-Key must contain 1 to 200 printable ASCII characters without surrounding whitespace", true
	case "idempotency_request_unsupported":
		return "This request cannot be safely fingerprinted for idempotency; use a supported JSON or multipart task request", true
	case "idempotency_key_conflict":
		return "Idempotency-Key conflicts with this request", true
	case "idempotency_in_progress":
		return "The request is in progress or requires funding reconciliation; retry only with the same Idempotency-Key and request", true
	case "idempotency_indeterminate", "idempotency_legacy_indeterminate":
		return "The funding or upstream outcome is uncertain; do not submit this operation with a new key", true
	case "idempotency_replay_unavailable":
		return "The accepted task result cannot be replayed; do not resubmit with a new key", true
	case "idempotency_storage_unavailable":
		return "Idempotency storage is unavailable; no new submission was started", true
	default:
		return "", false
	}
}
