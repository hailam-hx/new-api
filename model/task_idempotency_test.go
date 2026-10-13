package model_test

import (
	"bytes"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestTaskIdempotencyFingerprint(t *testing.T) {
	fingerprint := func(body, contentType, path string) (string, error) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", contentType)
		defer common.CleanupBodyStorage(c)
		return service.TaskIdempotencyFingerprint(c)
	}
	first, err := fingerprint(`{"model":"test","a":1,"b":{"x":false}}`, "application/json", "/v1/videos?a=1&b=2")
	require.NoError(t, err)
	second, err := fingerprint(" {\"b\":{\"x\":false},\"a\":1.0,\"model\":\"test\"} ", "application/json; charset=utf-8", "/v1/videos?b=2&a=1")
	require.NoError(t, err)
	assert.Equal(t, first, second)
	profile, err := fingerprint(`{"model":"test","a":1,"b":{"x":false}}`, `application/json; profile="https://example.invalid/profile"`, "/v1/videos?a=1&b=2")
	require.NoError(t, err)
	assert.NotEqual(t, first, profile, "nondefault media-type parameters can affect task driver semantics")
	acceptFingerprint := func(accept string) string {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewBufferString(`{"model":"test"}`))
		c.Request.Header.Set("Accept", accept)
		defer common.CleanupBodyStorage(c)
		value, err := service.TaskIdempotencyFingerprint(c)
		require.NoError(t, err)
		return value
	}
	assert.NotEqual(t, acceptFingerprint("application/json"), acceptFingerprint("text/event-stream"), "Accept is exposed to task driver hooks")
	for _, body := range []string{`{"value":9007199254740992}`, `{"value":9007199254740993}`} {
		current, err := fingerprint(body, "application/json", "/v1/videos")
		require.NoError(t, err)
		if first != "" {
			assert.NotEqual(t, first, current)
		}
		first = current
	}
	for _, pair := range [][2]string{{`{"n":1e4096}`, `{"n":10e4095}`}, {`{"n":1e-4096}`, `{"n":0.1e-4095}`}, {`{"n":-0}`, `{"n":0.0}`}} {
		a, err := fingerprint(pair[0], "application/json", "/v1/videos")
		require.NoError(t, err)
		b, err := fingerprint(pair[1], "application/json", "/v1/videos")
		require.NoError(t, err)
		assert.Equal(t, a, b)
	}

	for _, body := range []string{`{"a":1,"a":2}`, `{"a":1,"\u0061":2}`, `{"nested":{"x":1,"x":2}}`, `{"x":1} trailing`, `{"x":1e1000000000}`} {
		_, err := fingerprint(body, "application/json", "/v1/videos")
		require.Error(t, err, body)
	}

	_, err = fingerprint(strings.Repeat("[", 33)+"0"+strings.Repeat("]", 33), "application/json", "/v1/videos")
	require.Error(t, err)
	quoted, err := common.Marshal(map[string]any{"quoted": strings.Repeat("{[", 33) + `\"`})
	require.NoError(t, err)
	_, err = fingerprint(string(quoted), "application/json", "/v1/videos")
	require.NoError(t, err, "quoted braces and escaped quotes do not add JSON depth")

	multipartFingerprint := func(boundary, file string) string {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		require.NoError(t, writer.SetBoundary(boundary))
		require.NoError(t, writer.WriteField("model", "test"))
		part, err := writer.CreateFormFile("input_reference", "same.png")
		require.NoError(t, err)
		_, err = part.Write([]byte(file))
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		value, err := fingerprint(body.String(), writer.FormDataContentType(), "/v1/videos")
		require.NoError(t, err)
		return value
	}
	assert.Equal(t, multipartFingerprint("boundaryA", "bytesA"), multipartFingerprint("boundaryB", "bytesA"))
	assert.NotEqual(t, multipartFingerprint("boundaryA", "bytesA"), multipartFingerprint("boundaryA", "bytesB"))
}

func TestTaskIdempotencyDatabaseContract(t *testing.T) {
	for _, engine := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			var driver gorm.Dialector
			switch engine {
			case "sqlite":
				driver = sqlite.Open(filepath.Join(t.TempDir(), "idempotency.db") + "?_pragma=busy_timeout(5000)")
			case "mysql":
				dsn := os.Getenv("PHASE5_MYSQL_DSN")
				if dsn == "" {
					t.Skip("PHASE5_MYSQL_DSN not configured")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("PHASE5_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("PHASE5_POSTGRES_DSN not configured")
				}
				driver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			}
			db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: fmt.Sprintf("p5idem_%d_", time.Now().UnixNano())}})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(8)
			previous := model.DB
			previousLogDB := model.LOG_DB
			logDriver := driver
			if engine == "sqlite" {
				logDriver = sqlite.Open(filepath.Join(t.TempDir(), "log.db"))
			}
			logDB, err := gorm.Open(logDriver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: fmt.Sprintf("p5idem_log_%d_", time.Now().UnixNano())}})
			require.NoError(t, err)
			logSQL, err := logDB.DB()
			require.NoError(t, err)
			model.LOG_DB = logDB
			previousDialect, previousLogDialect := common.MainDatabaseType(), common.LogDatabaseType()
			common.SetDatabaseTypes(common.DatabaseType(engine), common.DatabaseType(engine))
			model.DB = db
			t.Cleanup(func() {
				model.DB = previous
				model.LOG_DB = previousLogDB
				common.SetDatabaseTypes(previousDialect, previousLogDialect)
				_ = db.Migrator().DropTable(&model.TaskIdempotency{}, &model.Task{})
				_ = logDB.Migrator().DropTable(&model.Log{})
				_ = logSQL.Close()
				_ = sqlDB.Close()
			})
			for range 2 {
				require.NoError(t, logDB.AutoMigrate(&model.Log{}))
			}
			// Fresh startup twice before simulating the prior task-only schema.
			for range 2 {
				require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TaskIdempotency{}))
			}
			require.NoError(t, db.Migrator().DropTable(&model.TaskIdempotency{}))
			// Upgrade from the pre-feature task schema, retaining a representative task.
			require.NoError(t, db.AutoMigrate(&model.Task{}))
			existing := &model.Task{TaskID: "existing", UserId: 1, TokenId: 7, Status: model.TaskStatusSuccess}
			require.NoError(t, db.Create(existing).Error)
			for range 2 {
				require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TaskIdempotency{}))
			}
			var retained model.Task
			require.NoError(t, db.First(&retained, existing.ID).Error)
			assert.Equal(t, "existing", retained.TaskID)
			var version string
			query := "SELECT version()"
			if engine == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Logf("database %s: %s", engine, version)
			const now int64 = 2000000000
			var owners atomic.Int64
			var group sync.WaitGroup
			claimErrors := make(chan error, 8)
			for range 8 {
				group.Go(func() {
					_, owned, err := model.AcquireTaskIdempotency(t.Context(), 1, 7, "operation", "key", "fingerprint", now)
					if owned {
						owners.Add(1)
					}
					claimErrors <- err
				})
			}
			group.Wait()
			close(claimErrors)
			for err := range claimErrors {
				require.NoError(t, err)
			}
			assert.Equal(t, int64(1), owners.Load())
			record, owned, err := model.AcquireTaskIdempotency(t.Context(), 1, 7, "operation", "key", "fingerprint", now)
			require.NoError(t, err)
			assert.False(t, owned)
			_, _, err = model.AcquireTaskIdempotency(t.Context(), 1, 7, "operation", "key", "changed", now)
			require.ErrorIs(t, err, model.ErrTaskIdempotencyConflict)
			_, _, err = model.AcquireTaskIdempotency(t.Context(), 2, 7, "operation", "key", "fingerprint", now)
			require.ErrorIs(t, err, model.ErrTaskIdempotencyConflict)
			for _, token := range []int{8, 9} {
				_, owned, err := model.AcquireTaskIdempotency(t.Context(), 1, token, "operation", "key", "fingerprint", now)
				require.NoError(t, err)
				assert.True(t, owned)
			}
			_, owned, err = model.AcquireTaskIdempotency(t.Context(), 1, 7, "different-operation", "key", "fingerprint", now)
			require.NoError(t, err)
			assert.True(t, owned)
			require.NoError(t, model.BeginTaskIdempotencyOutbound(t.Context(), record.ID, record.Nonce))
			task := &model.Task{TaskID: "public-id", UserId: 1, TokenId: 7, Status: model.TaskStatusSubmitted, CreatedAt: now}
			require.NoError(t, model.InsertTaskWithIdempotency(t.Context(), task, record.ID, record.Nonce))
			replay, owned, err := model.AcquireTaskIdempotency(t.Context(), 1, 7, "operation", "key", "fingerprint", now+1)
			require.NoError(t, err)
			assert.False(t, owned)
			assert.Equal(t, task.ID, replay.ResourceRecordID)
			// Expiration never releases active or indeterminate work.
			_, owned, err = model.AcquireTaskIdempotency(t.Context(), 1, 7, "operation", "key", "fingerprint", now+10*86400)
			require.NoError(t, err)
			assert.False(t, owned)
			require.NoError(t, db.Model(task).Updates(map[string]any{"status": model.TaskStatusSuccess, "finish_time": now}).Error)
			_, owned, err = model.AcquireTaskIdempotency(t.Context(), 1, 7, "operation", "key", "new-request", now+10*86400)
			require.NoError(t, err)
			assert.True(t, owned)
			uncertain, _, err := model.AcquireTaskIdempotency(t.Context(), 1, 7, "operation", "unknown", "fingerprint", now)
			require.NoError(t, err)
			require.NoError(t, model.BeginTaskIdempotencyOutbound(t.Context(), uncertain.ID, uncertain.Nonce))
			require.NoError(t, model.ReleaseTaskIdempotencyClaim(t.Context(), uncertain.ID, uncertain.Nonce))
			_, owned, err = model.AcquireTaskIdempotency(t.Context(), 1, 7, "operation", "unknown", "fingerprint", now+10*86400)
			require.NoError(t, err)
			assert.False(t, owned)
			stale, _, err := model.AcquireTaskIdempotency(t.Context(), 1, 7, "operation", "stale", "old-request", now)
			require.NoError(t, err)
			require.NoError(t, db.Model(stale).Update("expires_at", time.Now().Unix()-1).Error)
			replacement, owned, err := model.AcquireTaskIdempotency(t.Context(), 1, 7, "operation", "stale", "old-request", now+10*86400)
			require.NoError(t, err)
			assert.False(t, owned, "abandoned pre-outbound claims may retain funding and must not permit another reservation")
			assert.Equal(t, stale.Nonce, replacement.Nonce)
			require.Error(t, model.BeginTaskIdempotencyOutbound(t.Context(), stale.ID, stale.Nonce), "an expired actor must not start paid work")
			active, _, err := model.AcquireTaskIdempotency(t.Context(), 1, 7, "operation", "active", "fingerprint", now)
			require.NoError(t, err)
			require.NoError(t, model.BeginTaskIdempotencyOutbound(t.Context(), active.ID, active.Nonce))
			require.NoError(t, model.InsertTaskWithIdempotency(t.Context(), &model.Task{UserId: 1, TokenId: 7, Status: model.TaskStatusSubmitted}, active.ID, active.Nonce))
			failed, _, err := model.AcquireTaskIdempotency(t.Context(), 1, 7, "operation", "failed", "fingerprint", now)
			require.NoError(t, err)
			require.NoError(t, model.BeginTaskIdempotencyOutbound(t.Context(), failed.ID, failed.Nonce))
			require.NoError(t, model.InsertTaskWithIdempotency(t.Context(), &model.Task{UserId: 1, TokenId: 7, Status: model.TaskStatusFailure, FinishTime: now}, failed.ID, failed.Nonce))
			require.NoError(t, model.CleanupTaskIdempotency(t.Context(), now+20*86400))
			var remaining model.TaskIdempotency
			require.NoError(t, db.First(&remaining, stale.ID).Error)
			remaining = model.TaskIdempotency{}
			require.NoError(t, db.First(&remaining, active.ID).Error)
			remaining = model.TaskIdempotency{}
			require.NoError(t, db.First(&remaining, uncertain.ID).Error)
			require.ErrorIs(t, db.First(&model.TaskIdempotency{}, failed.ID).Error, gorm.ErrRecordNotFound)
			preflight, _, err := model.AcquireTaskIdempotency(t.Context(), 1, 7, "operation", "preflight", "fingerprint", now)
			require.NoError(t, err)
			require.NoError(t, model.ReleaseTaskIdempotencyClaim(t.Context(), preflight.ID, preflight.Nonce))
			_, owned, err = model.AcquireTaskIdempotency(t.Context(), 1, 7, "operation", "preflight", "fingerprint", now)
			require.NoError(t, err)
			assert.True(t, owned)

			blocked, _, err := model.AcquireTaskIdempotency(t.Context(), 1, 7, "operation", "write-failure", "fingerprint", now)
			require.NoError(t, err)
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register("fixture:outbound-write-failure", func(tx *gorm.DB) { tx.AddError(errors.New("fixture main database unavailable")) }))
			blockedContext, _ := gin.CreateTestContext(httptest.NewRecorder())
			blockedContext.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
			blockedInfo := &relaycommon.RelayInfo{TaskIdempotencyID: blocked.ID, TaskIdempotencyNonce: blocked.Nonce}
			require.Error(t, service.MarkTaskIdempotencyOutbound(blockedContext, blockedInfo))
			assert.False(t, blockedInfo.TaskIdempotencyOutbound, "a failed durable marker cannot authorize paid submission")
			require.NoError(t, db.Callback().Update().Remove("fixture:outbound-write-failure"))
			require.NoError(t, model.ReleaseTaskIdempotencyClaim(t.Context(), blocked.ID, blocked.Nonce))
			_, owned, err = model.AcquireTaskIdempotency(t.Context(), 1, 7, "operation", "write-failure", "fingerprint", now)
			require.NoError(t, err)
			assert.True(t, owned)
			rollback, _, err := model.AcquireTaskIdempotency(t.Context(), 1, 7, "operation", "rollback", "fingerprint", now)
			require.NoError(t, err)
			require.NoError(t, model.BeginTaskIdempotencyOutbound(t.Context(), rollback.ID, rollback.Nonce))
			var beforeTasks int64
			require.NoError(t, db.Model(&model.Task{}).Count(&beforeTasks).Error)
			require.NoError(t, db.Callback().Create().After("gorm:create").Register("fixture:task-persistence-failure", func(tx *gorm.DB) { tx.AddError(errors.New("fixture task persistence unavailable")) }))
			require.Error(t, model.InsertTaskWithIdempotency(t.Context(), &model.Task{UserId: 1, TokenId: 7, TaskID: "must-rollback"}, rollback.ID, rollback.Nonce))
			require.NoError(t, db.Callback().Create().Remove("fixture:task-persistence-failure"))
			var afterTasks int64
			require.NoError(t, db.Model(&model.Task{}).Count(&afterTasks).Error)
			assert.Equal(t, beforeTasks, afterTasks)
			retainedClaim, owned, err := model.AcquireTaskIdempotency(t.Context(), 1, 7, "operation", "rollback", "fingerprint", now)
			require.NoError(t, err)
			assert.False(t, owned)
			assert.Equal(t, model.TaskIdempotencyIndeterminate, retainedClaim.State)
			assert.Zero(t, retainedClaim.ResourceRecordID)

			// Exercise the real DFLOP descriptor without network I/O: provider
			// identity uses the claim namespace, never the caller's raw key.
			source, err := plugins.Source("dflop-tts")
			require.NoError(t, err)
			plugin, err := pluginruntime.CompilePlugin(source, pluginruntime.Options{})
			require.NoError(t, err)
			requestContext, _ := gin.CreateTestContext(httptest.NewRecorder())
			requestContext.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
			requestContext.Request.Header.Set("Idempotency-Key", "legacy-fixture-key")
			requestContext.Set("task_request", map[string]any{"model": "voice-tts-pro", "input": "Hello.", "async": true})
			identity := func(token int, id int64, nonce string) *relaycommon.RelayInfo {
				return &relaycommon.RelayInfo{UserId: 1, TokenId: token, TaskIdempotencyID: id, TaskIdempotencyNonce: nonce, OriginModelName: "voice-tts-pro", TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api.dflop.top", UpstreamModelName: "voice-tts-pro", ApiKey: "fixture-only"}}
			}
			firstInfo := identity(7, 100, "operation-one")
			require.NoError(t, service.PrepareDFLOPTaskBillingIdentity(requestContext, firstInfo, "dflop-tts"))
			for _, info := range []*relaycommon.RelayInfo{firstInfo, identity(7, 101, "operation-two"), identity(8, 102, "different-token")} {
				require.NoError(t, service.PrepareDFLOPTaskBillingIdentity(requestContext, info, "dflop-tts"))
				adaptor := taskplugin.New(plugin)
				adaptor.Init(info)
				require.Nil(t, adaptor.ValidateRequestAndSetAction(requestContext, info))
				outbound := &http.Request{Header: make(http.Header)}
				require.NoError(t, adaptor.BuildRequestHeader(requestContext, outbound, info))
				assert.Equal(t, info.RequestId, outbound.Header.Get("Idempotency-Key"))
				assert.NotEqual(t, "legacy-fixture-key", outbound.Header.Get("Idempotency-Key"))
				if info != firstInfo {
					assert.NotEqual(t, firstInfo.RequestId, info.RequestId)
				}
				assert.Equal(t, "legacy-fixture-key", requestContext.GetHeader("Idempotency-Key"))
			}
			legacy := firstInfo.DFLOPTaskLegacyRequestID
			require.NoError(t, logDB.Create(&model.Log{UserId: 1, TokenId: 9, RequestId: legacy, Content: "BILLING_RESERVATION_PENDING"}).Error)
			require.NoError(t, service.PrepareDFLOPTaskBillingIdentity(requestContext, identity(7, 103, "foreign-independent"), "dflop-tts"))
			require.NoError(t, logDB.Create(&model.Log{UserId: 1, TokenId: 7, RequestId: legacy, Content: "BILLING_RESERVATION_PENDING"}).Error)
			require.ErrorContains(t, service.PrepareDFLOPTaskBillingIdentity(requestContext, identity(7, 104, "own-legacy"), "dflop-tts"), "BILLING_RESERVATION_REPLAY")
			require.NoError(t, logDB.Create(&model.Log{UserId: 1, TokenId: 0, RequestId: legacy, Content: "BILLING_RESERVATION_PENDING"}).Error)
			require.ErrorContains(t, service.PrepareDFLOPTaskBillingIdentity(requestContext, identity(8, 105, "unknown-legacy"), "dflop-tts"), "BILLING_RESERVATION_REPLAY")
		})
	}
}
