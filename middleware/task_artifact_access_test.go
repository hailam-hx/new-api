package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMidjourneyImageCapabilityExpiryAndRouteBinding(t *testing.T) {
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "midjourney-bounded-preview"
	t.Cleanup(func() { common.CryptoSecret = previousSecret })
	const now int64 = 1_800_000_000
	for _, tc := range []struct {
		name    string
		expires int64
		valid   bool
	}{
		{"valid", now + 900, true},
		{"expired", now - 1, false},
		{"at expiry", now, false},
		{"too far future", now + 901, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			access, err := service.IssueTaskArtifactAccess("/mj/image/mj-one", "image:"+strconv.FormatInt(tc.expires, 10))
			require.NoError(t, err)
			assert.Equal(t, tc.valid, service.VerifyMidjourneyImageAccess(access, "mj-one", tc.expires, now))
			assert.False(t, service.VerifyMidjourneyImageAccess(access, "mj-two", tc.expires, now))
			assert.False(t, service.VerifyMidjourneyImageAccess(access, "mj-one", tc.expires+1, now))
			assert.False(t, service.VerifyTaskArtifactAccess(access, "mj-one", "image"))
		})
	}
}

func TestMidjourneyImageAuthRejectsMissingTamperedAndReplayedCapabilities(t *testing.T) {
	previousSecret, previousAddress := common.CryptoSecret, system_setting.ServerAddress
	common.CryptoSecret = "midjourney-route-preview"
	system_setting.ServerAddress = "https://gateway.example"
	t.Cleanup(func() { common.CryptoSecret = previousSecret; system_setting.ServerAddress = previousAddress })
	preview, err := service.BuildMidjourneyImageURL("mj-one")
	require.NoError(t, err)
	parsed, err := url.Parse(preview)
	require.NoError(t, err)
	expires, err := strconv.ParseInt(parsed.Query().Get("expires"), 10, 64)
	require.NoError(t, err)
	assert.True(t, service.VerifyMidjourneyImageAccess(parsed.Query().Get("access"), "mj-one", expires, common.GetTimestamp()))
	expiredAt, futureAt := common.GetTimestamp()-1, common.GetTimestamp()+1800
	expiredAccess, err := service.IssueTaskArtifactAccess("/mj/image/mj-one", "image:"+strconv.FormatInt(expiredAt, 10))
	require.NoError(t, err)
	futureAccess, err := service.IssueTaskArtifactAccess("/mj/image/mj-one", "image:"+strconv.FormatInt(futureAt, 10))
	require.NoError(t, err)
	router := gin.New()
	router.Use(redactTaskArtifactAccessQuery())
	router.GET("/mj/image/:id", MidjourneyImageAuth(), func(c *gin.Context) {
		assert.True(t, IsTaskArtifactAccess(c))
		assert.NotContains(t, c.Request.URL.RawQuery, "access")
		c.Status(http.StatusNoContent)
	})
	router.GET("/mj-fast/mj/image/:id", MidjourneyImageAuth(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	for _, tc := range []struct {
		name, path string
		status     int
	}{
		{"valid", parsed.RequestURI(), http.StatusNoContent},
		{"alias", "/mj-fast" + parsed.RequestURI(), http.StatusNoContent},
		{"missing", "/mj/image/mj-one", http.StatusUnauthorized},
		{"tampered", "/mj/image/mj-one?expires=" + strconv.FormatInt(expires, 10) + "&access=invalid", http.StatusNotFound},
		{"other task", "/mj/image/mj-two?" + parsed.RawQuery, http.StatusNotFound},
		{"duplicate expiry", parsed.RequestURI() + "&expires=" + strconv.FormatInt(expires, 10), http.StatusNotFound},
		{"unsigned expiry", "/mj/image/mj-one?access=" + parsed.Query().Get("access"), http.StatusNotFound},
		{"expired", "/mj/image/mj-one?expires=" + strconv.FormatInt(expiredAt, 10) + "&access=" + expiredAccess, http.StatusNotFound},
		{"future bound", "/mj/image/mj-one?expires=" + strconv.FormatInt(futureAt, 10) + "&access=" + futureAccess, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.path, nil))
			assert.Equal(t, tc.status, recorder.Code)
		})
	}
	common.CryptoSecret = "rotated-midjourney-preview"
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, parsed.RequestURI(), nil))
	assert.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestTaskArtifactAccessIsRedactedAndVerifiedBeforeHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "task-artifact-middleware-secret"
	t.Cleanup(func() { common.CryptoSecret = previousSecret })

	access, err := service.IssueTaskArtifactAccess("task-1", "video-main")
	require.NoError(t, err)

	router := gin.New()
	router.Use(redactTaskArtifactAccessQuery())
	router.GET(
		"/v1/tasks/:key/artifacts/:artifact_key/content",
		TokenOrTaskArtifactAccessAuth("key", "artifact_key"),
		func(c *gin.Context) {
			assert.True(t, IsTaskArtifactAccess(c))
			assert.NotContains(t, c.Request.URL.RawQuery, service.TaskArtifactAccessQueryParameter)
			assert.Equal(t, "kept", c.Query("keep"))
			c.Status(http.StatusNoContent)
		},
	)
	request := httptest.NewRequest(
		http.MethodGet,
		"/v1/tasks/task-1/artifacts/video-main/content?access="+urlQueryEscape(access)+"&keep=kept",
		nil,
	)
	request.RemoteAddr = "192.0.2.1:1234"
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestTaskArtifactAccessRejectsTamperedAndEmptyCapabilitiesAsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "task-artifact-middleware-reject-secret"
	t.Cleanup(func() { common.CryptoSecret = previousSecret })

	router := gin.New()
	router.Use(redactTaskArtifactAccessQuery())
	router.GET(
		"/v1/tasks/:key/artifacts/:artifact_key/content",
		TokenOrTaskArtifactAccessAuth("key", "artifact_key"),
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)

	for _, query := range []string{
		"?access=",
		"?access=invalid",
		"?access=first&access=second",
		"?access=" + strings.Repeat("x", 1024),
		"?access=%20" + strings.Repeat("A", 43) + "%20",
	} {
		request := httptest.NewRequest(
			http.MethodGet,
			"/v1/tasks/task-1/artifacts/video-main/content"+query,
			nil,
		)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusNotFound, recorder.Code)
	}
}

func TestTaskArtifactAccessLimiterDefaults(t *testing.T) {
	limits := system_setting.TaskArtifactAccessLimits{
		InvalidRatePerMinute: system_setting.DefaultTaskArtifactInvalidRateLimitPerMinute,
		GlobalConcurrency:    system_setting.DefaultTaskArtifactGlobalConcurrency,
		IPConcurrency:        system_setting.DefaultTaskArtifactIPConcurrency,
		ObjectConcurrency:    system_setting.DefaultTaskArtifactObjectConcurrency,
	}
	limiter := newTaskArtifactAccessLimiter(limits)
	now := time.Unix(1000, 0)
	releases := make([]func(), 0, limits.ObjectConcurrency)
	for i := 0; i < limits.ObjectConcurrency; i++ {
		release, ok := limiter.acquire("192.0.2.1", "task-1", "video")
		require.True(t, ok)
		releases = append(releases, release)
	}
	_, ok := limiter.acquire("192.0.2.2", "task-1", "video")
	assert.False(t, ok, "task+key concurrency is shared across IPs")
	for _, release := range releases {
		release()
	}

	rateLimiter := newTaskArtifactAccessLimiter(limits)
	for i := 0; i < limits.InvalidRatePerMinute; i++ {
		assert.True(t, rateLimiter.invalidAttempt(now, "192.0.2.10"))
	}
	assert.False(t, rateLimiter.invalidAttempt(now, "192.0.2.10"))
	assert.True(t, rateLimiter.invalidAttempt(now.Add(time.Minute), "192.0.2.10"))
}

func TestRedactTaskArtifactAccessAlsoCoversLegacyVideoRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(redactTaskArtifactAccessQuery())
	router.GET("/v1/videos/:task_id/content", func(c *gin.Context) {
		assert.NotContains(t, c.Request.URL.RawQuery, "access")
		assert.NotContains(t, c.Request.RequestURI, "secret-capability")
		assert.Equal(t, "ok", c.Query("keep"))
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(
		http.MethodGet,
		"/v1/videos/task-1/content?access=secret-capability&keep=ok",
		nil,
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestSetUpLoggerNeverWritesTaskArtifactAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousWriter := gin.DefaultWriter
	var output bytes.Buffer
	gin.DefaultWriter = &output
	t.Cleanup(func() { gin.DefaultWriter = previousWriter })

	router := gin.New()
	SetUpLogger(router)
	router.GET("/v1/tasks/:key/artifacts/:artifact_key/content", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(
		http.MethodGet,
		"/v1/tasks/task-1/artifacts/video/content?access=never-log-this&keep=ok",
		nil,
	)
	router.ServeHTTP(httptest.NewRecorder(), request)

	assert.False(t, strings.Contains(output.String(), "never-log-this"))
}

func urlQueryEscape(value string) string {
	replacer := strings.NewReplacer("+", "%2B", "=", "%3D")
	return replacer.Replace(value)
}
