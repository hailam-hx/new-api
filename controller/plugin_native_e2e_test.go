package controller

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type nativeRouteBilling struct {
	events      []string
	preConsumed int
	userID      int
	settled     bool
}

func TestDFLOPAudioSpeechProductionTaskSubmissionAndPolling(t *testing.T) {
	cases := []struct {
		name           string
		terminal       string
		status         model.TaskStatus
		wantCharacters float64
		clientGone     bool
		lostResponse   bool
	}{
		{"success", `{"id":"fake-task-1","model":"voice-tts-pro","status":"succeeded","characters":6,"duration_sec":"1.20","audio_url":"https://example.invalid/audio.mp3"}`, model.TaskStatusSuccess, 6, false, false},
		{"failure", `{"id":"fake-task-1","model":"voice-tts-pro","status":"failed","error":{"message":"render failed"}}`, model.TaskStatusFailure, 6, false, false},
		{"missing characters", `{"id":"fake-task-1","model":"voice-tts-pro","status":"succeeded","audio_url":"https://example.invalid/audio.mp3"}`, model.TaskStatusSuccess, 6, false, false},
		{"explicit zero", `{"id":"fake-task-1","model":"voice-tts-pro","status":"succeeded","characters":0,"audio_url":"https://example.invalid/audio.mp3"}`, model.TaskStatusSuccess, 0, false, false},
		{"client disconnect", `{"id":"fake-task-1","model":"voice-tts-pro","status":"succeeded","characters":6,"audio_url":"https://example.invalid/audio.mp3"}`, model.TaskStatusSuccess, 6, true, false},
		{"lost submit response", `{"id":"fake-task-1","model":"voice-tts-pro","status":"succeeded","characters":6,"audio_url":"https://example.invalid/audio.mp3"}`, model.TaskStatusSuccess, 6, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			service.InitHttpClient()
			previousDB, previousLogDB := model.DB, model.LOG_DB
			previousMemoryCache, previousBatchUpdate := common.MemoryCacheEnabled, common.BatchUpdateEnabled
			previousLogConsume, previousRedis := common.LogConsumeEnabled, common.RedisEnabled
			database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, database.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Task{}, &model.Log{}))
			model.DB, model.LOG_DB = database, database
			common.MemoryCacheEnabled, common.BatchUpdateEnabled = false, false
			common.LogConsumeEnabled, common.RedisEnabled = true, false
			previousBilling := config.GlobalConfig.Get("billing_setting")
			previousBillingMap, err := config.ConfigToMap(previousBilling)
			require.NoError(t, err)
			previousRatios := ratio_setting.ModelRatio2JSONString()
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"voice-tts-pro":1}`))
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				billing_setting.PluginBillingExprOption: `{"dflop-tts::voice-tts-pro":"u(\"characters\") * 0.001"}`,
			}))
			t.Cleanup(func() {
				model.DB, model.LOG_DB = previousDB, previousLogDB
				common.MemoryCacheEnabled, common.BatchUpdateEnabled = previousMemoryCache, previousBatchUpdate
				common.LogConsumeEnabled, common.RedisEnabled = previousLogConsume, previousRedis
				_ = ratio_setting.UpdateModelRatioByJSONString(previousRatios)
				_ = config.UpdateConfigFromMap(previousBilling, previousBillingMap)
			})
			require.NoError(t, database.Create(&model.User{Id: 17, Username: "speech-user", Group: "default", Quota: 1_000_000}).Error)
			require.NoError(t, database.Create(&model.Token{Id: 27, UserId: 17, Key: "speech-test-token", Status: 1, RemainQuota: 1_000_000}).Error)

			var submitCalls atomic.Int32
			var queryCalls atomic.Int32
			var firstIdempotencyKey string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/v1/audio/speech":
					attempt := submitCalls.Add(1)
					key := r.Header.Get("Idempotency-Key")
					assert.NotEmpty(t, key)
					if attempt == 1 {
						firstIdempotencyKey = key
					} else {
						assert.Equal(t, firstIdempotencyKey, key)
					}
					body, readErr := io.ReadAll(r.Body)
					assert.NoError(t, readErr)
					assert.JSONEq(t, `{"model":"voice-tts-pro","input":"Hello.","async":true}`, string(body))
					if tc.lostResponse && attempt == 1 {
						connection, _, hijackErr := w.(http.Hijacker).Hijack()
						assert.NoError(t, hijackErr)
						_ = connection.Close()
						return
					}
					_, _ = io.WriteString(w, `{"id":"fake-task-1","model":"voice-tts-pro","status":"pending","characters":6}`)
				case r.Method == http.MethodGet && r.URL.Path == "/v1/audio/speech/fake-task-1":
					if queryCalls.Add(1) == 1 {
						_, _ = io.WriteString(w, `{"id":"fake-task-1","model":"voice-tts-pro","status":"pending"}`)
					} else {
						_, _ = io.WriteString(w, tc.terminal)
					}
				default:
					http.NotFound(w, r)
				}
			}))
			defer upstream.Close()
			channel := model.Channel{Type: constant.ChannelTypeOpenAI, Name: "fake-dflop", Key: "sk-fake", BaseURL: &upstream.URL, Status: common.ChannelStatusEnabled, Models: "voice-tts-pro", Group: "default"}
			channel.SetSetting(dto.ChannelSettings{TaskPluginKey: "dflop-tts"})
			require.NoError(t, database.Create(&channel).Error)

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", bytes.NewBufferString(`{"model":"voice-tts-pro","input":"Hello.","async":true}`))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Request.Header.Set("Idempotency-Key", "client-intent-1")
			common.SetContextKey(c, constant.ContextKeyUserId, 17)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
			common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
			common.SetContextKey(c, constant.ContextKeyUserQuota, 1_000_000)
			common.SetContextKey(c, constant.ContextKeyTokenId, 27)
			common.SetContextKey(c, constant.ContextKeyTokenKey, "speech-test-token")
			common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
			middleware.PinTaskPluginEndpoint()(c)
			require.False(t, c.IsAborted(), recorder.Body.String())
			middleware.PrepareTaskPluginEndpoint()(c)
			require.False(t, c.IsAborted(), recorder.Body.String())
			require.Nil(t, middleware.SetupContextForSelectedChannel(c, &channel, "voice-tts-pro"))

			var outcome *taskSubmissionOutcome
			var taskErr *taskdto.TaskError
			deps := defaultPluginProtocolBridgeDeps()
			deps.submit = func(c *gin.Context, info *relaycommon.RelayInfo) (*taskSubmissionOutcome, *taskdto.TaskError) {
				require.NotNil(t, info.TaskRelayInfo)
				assert.Equal(t, channel.Id, info.LockedChannel.(*model.Channel).Id)
				info.PublicTaskID = "task_speech_public"
				outcome, taskErr = executeTaskSubmissionWith(c, info, relay.RelayTaskSubmit)
				return outcome, taskErr
			}
			pinnedValue, found := c.Get(pluginruntime.ContextKeyPinnedEndpoint)
			require.True(t, found)
			pinned, ok := pinnedValue.(pluginruntime.PinnedEndpoint)
			require.True(t, ok)
			clientContext, cancelClient := context.WithCancel(c.Request.Context())
			c.Request = c.Request.WithContext(clientContext)
			defer cancelClient()
			if tc.clientGone {
				cancelClient()
			}
			serveTaskPluginAudioSpeech(c, pinned, deps)
			require.Nil(t, taskErr, "%+v", taskErr)
			require.NotNil(t, outcome)
			require.NotNil(t, outcome.RelayInfo.Billing)
			assert.Equal(t, service.BillingSourceWallet, outcome.RelayInfo.BillingSource)
			expectedSubmitCalls := int32(1)
			if tc.lostResponse {
				expectedSubmitCalls = 2
			}
			assert.Equal(t, expectedSubmitCalls, submitCalls.Load())
			if tc.clientGone {
				assert.Empty(t, recorder.Body.String())
			} else {
				assert.Contains(t, recorder.Body.String(), "task_speech_public")
			}
			assert.NotContains(t, recorder.Body.String(), "fake-task-1")

			var persisted model.Task
			require.NoError(t, database.Where("task_id = ?", "task_speech_public").First(&persisted).Error)
			assert.Equal(t, "fake-task-1", persisted.PrivateData.UpstreamTaskID)
			assert.Equal(t, "dflop-tts", string(persisted.Platform))
			previousAdaptorFactory := service.GetTaskAdaptorFunc
			service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor { return relay.GetTaskAdaptor(platform) }
			defer func() { service.GetTaskAdaptorFunc = previousAdaptorFactory }()
			var completionCharacters any
			for range 2 {
				service.DispatchPlatformUpdate(t.Context(), persisted.Platform, map[int][]string{channel.Id: {"fake-task-1"}}, map[string]*model.Task{"fake-task-1": &persisted})
				completionCharacters = persisted.PrivateData.BillingContext.TieredSnapshot.UsageFacts["characters"]
				require.NoError(t, database.Where("task_id = ?", "task_speech_public").First(&persisted).Error)
			}
			assert.Equal(t, tc.status, persisted.Status)
			if tc.status == model.TaskStatusFailure || tc.wantCharacters == 0 {
				assert.Zero(t, persisted.Quota)
			} else {
				assert.Equal(t, outcome.Task.Quota, persisted.Quota)
			}
			assert.Equal(t, int32(2), queryCalls.Load())
			assert.Equal(t, tc.wantCharacters, completionCharacters)
			assert.Equal(t, float64(6), persisted.PrivateData.BillingContext.TieredSnapshot.UsageFacts["characters"], "the persisted snapshot retains the submission estimate")
			assert.Equal(t, expectedSubmitCalls, submitCalls.Load())
			var chargedUser model.User
			require.NoError(t, database.First(&chargedUser, 17).Error)
			assert.Equal(t, 1_000_000-persisted.Quota, chargedUser.Quota)
			var chargedToken model.Token
			require.NoError(t, database.First(&chargedToken, 27).Error)
			assert.Equal(t, 1_000_000-persisted.Quota, chargedToken.RemainQuota)
			var consumeLogs []model.Log
			require.NoError(t, database.Where("user_id = ? AND type = ?", 17, model.LogTypeConsume).Find(&consumeLogs).Error)
			require.NotEmpty(t, consumeLogs)
			assert.Equal(t, "voice-tts-pro", consumeLogs[0].ModelName)
			assert.Equal(t, channel.Id, consumeLogs[0].ChannelId)
			assert.Contains(t, consumeLogs[0].Other, "task_speech_public")
			if tc.name == "success" {
				for _, ownership := range []struct {
					userID int
					status int
				}{
					{17, http.StatusOK},
					{18, http.StatusNotFound},
				} {
					queryRecorder := httptest.NewRecorder()
					queryContext, _ := gin.CreateTestContext(queryRecorder)
					queryContext.Request = httptest.NewRequest(http.MethodGet, "/v1/audio/speech/task_speech_public", nil)
					queryContext.Params = gin.Params{{Key: "task_id", Value: "task_speech_public"}}
					common.SetContextKey(queryContext, constant.ContextKeyUserId, ownership.userID)
					RetrieveTaskPluginAudioSpeech(queryContext)
					assert.Equal(t, ownership.status, queryRecorder.Code)
					assert.NotContains(t, queryRecorder.Body.String(), "fake-task-1")
				}
			}
		})
	}
}
func (b *nativeRouteBilling) Settle(int) error {
	b.events = append(b.events, "settle")
	b.settled = true
	return nil
}

func (b *nativeRouteBilling) Refund(*gin.Context) {
	b.events = append(b.events, "refund")
	if !b.settled && b.preConsumed > 0 {
		_ = model.IncreaseUserQuota(b.userID, b.preConsumed, true)
		b.preConsumed = 0
	}
}

func (b *nativeRouteBilling) NeedsRefund() bool {
	return !b.settled && b.preConsumed > 0
}

func (b *nativeRouteBilling) GetPreConsumedQuota() int {
	return b.preConsumed
}

func (b *nativeRouteBilling) Reserve(quota int) error {
	b.events = append(b.events, "reserve")
	if err := model.DecreaseUserQuota(b.userID, quota, true); err != nil {
		return err
	}
	b.preConsumed = quota
	return nil
}

func TestKlingNativeRouteSubmitPollSettleAndQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousMemoryCache := common.MemoryCacheEnabled
	previousBatchUpdate := common.BatchUpdateEnabled
	previousLogConsume := common.LogConsumeEnabled
	previousRedisEnabled := common.RedisEnabled
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.Channel{}, &model.Task{}, &model.Log{}))
	model.DB = database
	model.LOG_DB = database
	common.MemoryCacheEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = false
	common.RedisEnabled = false
	previousModelRatios := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"kling-v1":1}`))
	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.MemoryCacheEnabled = previousMemoryCache
		common.BatchUpdateEnabled = previousBatchUpdate
		common.LogConsumeEnabled = previousLogConsume
		common.RedisEnabled = previousRedisEnabled
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousModelRatios))
	})
	require.NoError(t, database.Create(&model.User{
		Id:       7,
		Username: "native-route-user",
		Group:    "default",
		Quota:    1_000_000,
	}).Error)

	var submitCalls atomic.Int32
	var queryCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/kling/v1/videos/text2video":
			submitCalls.Add(1)
			body, readErr := io.ReadAll(r.Body)
			if !assert.NoError(t, readErr) {
				http.Error(w, "read request", http.StatusInternalServerError)
				return
			}
			assert.Contains(t, string(body), `"model_name":"kling-v1"`)
			_, _ = io.WriteString(w, `{"code":0,"message":"","data":{"task_id":"kling-private-1","task_status":"submitted"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/kling/v1/videos/text2video/kling-private-1":
			queryCalls.Add(1)
			_, _ = io.WriteString(w, `{"code":0,"message":"","data":{"task_id":"kling-private-1","task_status":"succeed","task_status_msg":"","task_result":{"videos":[{"id":"video-private","url":"https://cdn.example/video.mp4","duration":"5"}]},"final_unit_deduction":"1"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	channel := model.Channel{
		Type:    constant.ChannelTypeKling,
		Name:    "kling-native-e2e",
		Key:     "sk-test",
		BaseURL: &upstream.URL,
		Status:  common.ChannelStatusEnabled,
		Models:  "kling-v1",
		Group:   "default",
	}
	require.NoError(t, database.Create(&channel).Error)

	generation := pluginruntime.DefaultRegistry.Generation()
	require.NotNil(t, generation)
	submitBinding, found := generation.LookupDeclaredRoute(http.MethodPost, "/kling/v1/videos/text2video")
	require.True(t, found)
	require.Equal(t, "kling", submitBinding.Plugin.Meta.Key)

	submitRecorder := httptest.NewRecorder()
	submitContext, _ := gin.CreateTestContext(submitRecorder)
	submitContext.Request = httptest.NewRequest(
		http.MethodPost,
		"/kling/v1/videos/text2video",
		bytes.NewBufferString(`{"model_name":"kling-v1","prompt":"a lighthouse"}`),
	)
	submitContext.Request.Header.Set("Content-Type", "application/json")
	submitContext.Set(pluginruntime.ContextKeyPinnedRoute, pluginruntime.PinnedRoute{
		Generation: generation,
		Plugin:     submitBinding.Plugin,
		Route:      submitBinding.Route,
	})
	common.SetContextKey(submitContext, constant.ContextKeyUserId, 7)
	common.SetContextKey(submitContext, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(submitContext, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(submitContext, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(submitContext, constant.ContextKeyUserQuota, 1_000_000)

	middleware.PrepareTaskPluginRoute()(submitContext)
	require.False(t, submitContext.IsAborted(), submitRecorder.Body.String())
	require.Equal(t, "kling-v1", submitContext.GetString("resolved_task_model"))
	require.Equal(t, "text_to_video", submitContext.GetString("task_action"))
	require.Nil(t, middleware.SetupContextForSelectedChannel(submitContext, &channel, "kling-v1"))

	billing := &nativeRouteBilling{userID: 7}
	relayInfo := &relaycommon.RelayInfo{
		UserId:          7,
		UserGroup:       "default",
		UsingGroup:      "default",
		UserQuota:       1_000_000,
		TokenGroup:      "default",
		OriginModelName: "kling-v1",
		Billing:         billing,
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			Action:        submitContext.GetString("task_action"),
			PublicTaskID:  "task_kling_public",
			LockedChannel: &channel,
		},
	}

	outcome, taskErr := executeTaskSubmissionWith(submitContext, relayInfo, relay.RelayTaskSubmit)
	require.Nil(t, taskErr)
	require.NotNil(t, outcome)
	require.Equal(t, []string{"reserve", "settle"}, billing.events)
	require.False(t, submitContext.Writer.Written())

	presentTaskSubmission(submitContext, outcome)
	require.Equal(t, http.StatusOK, submitRecorder.Code)
	assert.Contains(t, submitRecorder.Body.String(), `"task_id":"task_kling_public"`)
	assert.NotContains(t, submitRecorder.Body.String(), "kling-private-1")
	assert.Equal(t, int32(1), submitCalls.Load())

	var persisted model.Task
	require.NoError(t, database.Where("task_id = ?", "task_kling_public").First(&persisted).Error)
	assert.Equal(t, constant.TaskPlatform("kling"), persisted.Platform)
	assert.Equal(t, "kling-private-1", persisted.PrivateData.UpstreamTaskID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusNotStart), persisted.Status)

	previousAdaptorFactory := service.GetTaskAdaptorFunc
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		return relay.GetTaskAdaptor(platform)
	}
	t.Cleanup(func() { service.GetTaskAdaptorFunc = previousAdaptorFactory })
	service.DispatchPlatformUpdate(
		context.Background(),
		persisted.Platform,
		map[int][]string{channel.Id: {"kling-private-1"}},
		map[string]*model.Task{"kling-private-1": &persisted},
	)

	require.NoError(t, database.Where("task_id = ?", "task_kling_public").First(&persisted).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), persisted.Status)
	assert.Equal(t, "100%", persisted.Progress)
	assert.Equal(t, 1, persisted.Quota)
	assert.Equal(t, int32(1), queryCalls.Load())
	var settledUser model.User
	require.NoError(t, database.First(&settledUser, 7).Error)
	assert.Equal(t, 999_999, settledUser.Quota)

	queryBinding, found := generation.LookupDeclaredRoute(http.MethodGet, "/kling/v1/videos/text2video/:task_id")
	require.True(t, found)
	queryRecorder := httptest.NewRecorder()
	queryContext, _ := gin.CreateTestContext(queryRecorder)
	queryContext.Request = httptest.NewRequest(http.MethodGet, "/kling/v1/videos/text2video/task_kling_public", nil)
	queryContext.Params = gin.Params{{Key: "task_id", Value: "task_kling_public"}}
	queryContext.Set(pluginruntime.ContextKeyPinnedRoute, pluginruntime.PinnedRoute{
		Generation: generation,
		Plugin:     queryBinding.Plugin,
		Route:      queryBinding.Route,
	})
	common.SetContextKey(queryContext, constant.ContextKeyUserId, 7)

	middleware.PrepareTaskPluginRoute()(queryContext)

	require.True(t, queryContext.IsAborted())
	require.Equal(t, http.StatusOK, queryRecorder.Code)
	assert.Contains(t, queryRecorder.Body.String(), `"task_id":"task_kling_public"`)
	assert.Contains(t, queryRecorder.Body.String(), `"task_status":"succeed"`)
	assert.NotContains(t, queryRecorder.Body.String(), "kling-private-1")
	assert.NotContains(t, queryRecorder.Body.String(), upstream.URL)
}
