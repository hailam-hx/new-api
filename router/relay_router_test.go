package router

import (
	"bytes"
	"context"
	"fmt"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListModelsSupportsOpenAIAndGeminiAuthentication(t *testing.T) {
	setupRelayRouterTestDB(t)

	user := model.User{
		Username: "models-user",
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    100,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		UserId:         user.Id,
		Key:            "modelstestkey",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		UnlimitedQuota: true,
	}).Error)

	engine := gin.New()
	SetRelayRouter(engine)

	tests := []struct {
		name           string
		path           string
		headerName     string
		expectedObject string
		expectedField  string
	}{
		{
			name:           "OpenAI bearer token",
			path:           "/v1/models",
			headerName:     "Authorization",
			expectedObject: "list",
			expectedField:  "data",
		},
		{
			name:          "Gemini API key header",
			path:          "/v1/models",
			headerName:    "x-goog-api-key",
			expectedField: "models",
		},
		{
			name:          "Gemini API key query",
			path:          "/v1/models?key=modelstestkey",
			expectedField: "models",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			if test.headerName != "" {
				value := "modelstestkey"
				if test.headerName == "Authorization" {
					value = "Bearer " + value
				}
				request.Header.Set(test.headerName, value)
			}

			engine.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
			var payload map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
			assert.Contains(t, payload, test.expectedField)
			assert.NotContains(t, payload, "error")
			if test.expectedObject != "" {
				assert.Equal(t, test.expectedObject, payload["object"])
			}
		})
	}
}

func setupRelayRouterTestDB(t *testing.T) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	originalIsMasterNode := common.IsMasterNode
	originalRedisEnabled := common.RedisEnabled
	originalSQLitePath := common.SQLitePath
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")

	common.IsMasterNode = false
	common.RedisEnabled = false
	common.SQLitePath = fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, os.Setenv("SQL_DSN", "local"))
	require.NoError(t, model.InitDB())
	model.LOG_DB = model.DB
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.Ability{}))

	t.Cleanup(func() {
		if sqlDB, err := model.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
		common.IsMasterNode = originalIsMasterNode
		common.RedisEnabled = originalRedisEnabled
		common.SQLitePath = originalSQLitePath
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		if hadSQLDSN {
			require.NoError(t, os.Setenv("SQL_DSN", originalSQLDSN))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
	})
}

func TestDeveloperAPIRoutesRequireBearer(t *testing.T) {
	setupRelayRouterTestDB(t)
	engine := gin.New()
	SetRelayRouter(engine)
	for _, path := range []string{"/v1/catalog", "/v1/key/balance"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			assert.Equal(t, http.StatusUnauthorized, response.Code)
			assert.Contains(t, response.Body.String(), "error")
		})
	}
}

func setupDeveloperAPITestDB(t *testing.T, dialect string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	if dialect == "mysql" && os.Getenv("PHASE2_MYSQL_DSN") == "" {
		t.Skip("PHASE2_MYSQL_DSN requires an isolated Phase 2 test database")
	}
	if dialect == "postgres" && os.Getenv("PHASE2_POSTGRES_DSN") == "" {
		t.Skip("PHASE2_POSTGRES_DSN requires an isolated Phase 2 test database")
	}
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	oldRedis := common.RedisEnabled
	oldCurrency := *operation_setting.GetGeneralSetting()
	oldGroupRatios := ratio_setting.GroupRatio2JSONString()
	oldGroups := setting.UserUsableGroups2JSONString()
	oldAPIEnable := common.GlobalApiRateLimitEnable
	common.RedisEnabled = false
	common.GlobalApiRateLimitEnable = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	oldMaster, oldSQLitePath := common.IsMasterNode, common.SQLitePath
	oldSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")
	oldLogDSN, hadLogDSN := os.LookupEnv("LOG_SQL_DSN")
	testDSN := "local"
	switch dialect {
	case "sqlite":
		common.SQLitePath = fmt.Sprintf("file:phase2_%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	case "mysql":
		testDSN = os.Getenv("PHASE2_MYSQL_DSN")
	case "postgres":
		testDSN = os.Getenv("PHASE2_POSTGRES_DSN")
	default:
		t.Fatalf("unsupported database %s", dialect)
	}
	if testDSN == "" {
		t.Skip("PHASE2_" + strings.ToUpper(dialect) + "_DSN requires an isolated Phase 2 test database")
	}
	common.IsMasterNode = false
	require.NoError(t, os.Setenv("SQL_DSN", testDSN))
	require.NoError(t, os.Unsetenv("LOG_SQL_DSN"))
	require.NoError(t, model.InitDB())
	db := model.DB
	model.DB, model.LOG_DB = db, db
	tables := []any{&model.User{}, &model.Token{}, &model.Ability{}, &model.Channel{}, &model.Model{}, &model.Vendor{}, &model.Log{}}
	for _, table := range tables {
		require.False(t, db.Migrator().HasTable(table), "use an empty isolated database")
	}
	require.NoError(t, db.AutoMigrate(tables...))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":0.5}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP","retired":"Retired","auto":"Auto"}`))
	currency := operation_setting.GetGeneralSetting()
	currency.QuotaDisplayType = operation_setting.QuotaDisplayTypeCustom
	currency.CustomCurrencySymbol = "积分"
	currency.CustomCurrencyExchangeRate = 2
	t.Cleanup(func() {
		require.NoError(t, db.Migrator().DropTable(tables...))
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
		common.IsMasterNode, common.SQLitePath = oldMaster, oldSQLitePath
		if hadSQLDSN {
			require.NoError(t, os.Setenv("SQL_DSN", oldSQLDSN))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
		if hadLogDSN {
			require.NoError(t, os.Setenv("LOG_SQL_DSN", oldLogDSN))
		} else {
			require.NoError(t, os.Unsetenv("LOG_SQL_DSN"))
		}
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLog)
		common.RedisEnabled = oldRedis
		common.GlobalApiRateLimitEnable = oldAPIEnable
		*operation_setting.GetGeneralSetting() = oldCurrency
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatios))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldGroups))
	})
}

func developerAPIRequest(engine *gin.Engine, path, bearer string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.RemoteAddr = "203.0.113.10:4567"
	if bearer != "" {
		request.Header.Set("Authorization", bearer)
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	return response
}

func TestDeveloperAPIAuthenticationAndBalanceDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			setupDeveloperAPITestDB(t, dialect)
			user := model.User{Username: "phase2-owner", Status: common.UserStatusEnabled, Group: "default", Quota: 0, AuthVersion: 1}
			require.NoError(t, model.DB.Create(&user).Error)
			engine := gin.New()
			engine.Use(middleware.RequestId())
			require.NoError(t, engine.SetTrustedProxies(nil))
			SetRelayRouter(engine)
			for i, test := range []struct {
				name             string
				status           int
				remain, used     int
				unlimited        bool
				expiry           int64
				ips, group       string
				banned           bool
				wantStatus       int
				total, remaining any
				usedDisplay      string
			}{
				{name: "limited", status: 1, remain: 750000, used: 250000, expiry: -1, wantStatus: 200, total: "4", remaining: "3", usedDisplay: "1"},
				{name: "negative allowance", status: 1, remain: -250000, used: 500000, expiry: -1, wantStatus: 200, total: "1", remaining: "-1", usedDisplay: "2"},
				{name: "zero", status: 1, remain: 0, used: 500000, expiry: -1, wantStatus: 200, total: "2", remaining: "0", usedDisplay: "2"},
				{name: "exhausted", status: 4, remain: 0, used: 250000, expiry: -1, wantStatus: 200, total: "1", remaining: "0", usedDisplay: "1"},
				{name: "unlimited", status: 1, unlimited: true, used: 250000, expiry: -1, wantStatus: 200, usedDisplay: "1"},
				{name: "expiry", status: 1, remain: 500000, expiry: 2000000000, wantStatus: 200, total: "2", remaining: "2", usedDisplay: "0"},
				{name: "expired timestamp", status: 1, remain: 500000, expiry: 1, wantStatus: 401},
				{name: "expired status", status: 3, remain: 500000, expiry: -1, wantStatus: 401},
				{name: "disabled", status: 2, remain: 500000, expiry: -1, wantStatus: 401},
				{name: "unknown status", status: 9, remain: 500000, expiry: -1, wantStatus: 401},
				{name: "allowed IP", status: 1, remain: 500000, expiry: -1, ips: "203.0.113.0/24", wantStatus: 200, total: "2", remaining: "2", usedDisplay: "0"},
				{name: "denied IP", status: 1, remain: 500000, expiry: -1, ips: "198.51.100.0/24", wantStatus: 403},
				{name: "unauthorized group", status: 1, remain: 500000, expiry: -1, group: "private", wantStatus: 403},
				{name: "deprecated group", status: 1, remain: 500000, expiry: -1, group: "retired", wantStatus: 403},
				{name: "banned user", status: 1, remain: 500000, expiry: -1, banned: true, wantStatus: 403},
			} {
				t.Run(test.name, func(t *testing.T) {
					token := model.Token{UserId: user.Id, Key: fmt.Sprintf("phase2key%d", i), Status: test.status, RemainQuota: test.remain, UsedQuota: test.used, UnlimitedQuota: test.unlimited, ExpiredTime: test.expiry, AccessedTime: 123, Group: test.group}
					if test.ips != "" {
						token.AllowIps = &test.ips
					}
					require.NoError(t, model.DB.Create(&token).Error)
					if test.banned {
						require.NoError(t, model.DB.Model(&user).Update("status", common.UserStatusDisabled).Error)
					}
					for _, path := range []string{"/v1/key/balance", "/v1/catalog"} {
						response := developerAPIRequest(engine, path, "Bearer sk-"+token.Key)
						require.Equal(t, test.wantStatus, response.Code, response.Body.String())
						require.NotEmpty(t, response.Header().Get(common.RequestIdKey))
						assert.Contains(t, response.Header().Get("Cache-Control"), "no-store")
						if test.wantStatus != 200 {
							assert.Contains(t, response.Body.String(), `"error"`)
							continue
						}
						if path != "/v1/key/balance" {
							continue
						}
						var body map[string]any
						require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
						assert.Equal(t, "key.balance", body["object"])
						assert.Equal(t, "points", body["unit"])
						assert.Equal(t, test.total, body["total"])
						assert.Equal(t, test.usedDisplay, body["used"])
						assert.Equal(t, test.remaining, body["remaining"])
						assert.Equal(t, test.unlimited, body["unlimited"])
						if test.expiry == -1 {
							assert.Nil(t, body["expires_at"])
						} else {
							assert.Equal(t, float64(test.expiry), body["expires_at"])
						}
						for _, secret := range []string{token.Key, "remain_quota", "used_quota", "user_id", "token_id", "allow_ips"} {
							assert.NotContains(t, response.Body.String(), secret)
						}
					}
					var after model.Token
					require.NoError(t, model.DB.First(&after, token.Id).Error)
					assert.Equal(t, test.remain, after.RemainQuota)
					assert.Equal(t, test.used, after.UsedQuota)
					assert.Equal(t, int64(123), after.AccessedTime)
					assert.Equal(t, test.status, after.Status)
					if test.banned {
						require.NoError(t, model.DB.Model(&user).Update("status", common.UserStatusEnabled).Error)
					}
				})
			}
			for _, authorization := range []string{"", "Basic phase2key0", "phase2key0", "Bearer invalid", "Bearer sk-", "Bearer phase2key0-extra", "Bearer phase2key0 other"} {
				for _, path := range []string{"/v1/catalog", "/v1/key/balance"} {
					response := developerAPIRequest(engine, path, authorization)
					assert.Equal(t, http.StatusUnauthorized, response.Code, response.Body.String())
				}
			}
			for _, path := range []string{"/v1/catalog?key=phase2key0", "/v1/key/balance?key=phase2key0"} {
				request := httptest.NewRequest(http.MethodGet, path, nil)
				request.Header.Set("x-api-key", "phase2key0")
				request.Header.Set("Cookie", "session=phase2key0")
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				assert.Equal(t, http.StatusUnauthorized, response.Code)
			}
			// A caller cannot switch the authenticated key through query parameters.
			response := developerAPIRequest(engine, "/v1/key/balance?token_id=2&user_id=999&key=phase2key1", "Bearer phase2key0")
			assert.Equal(t, http.StatusOK, response.Code)
			assert.Contains(t, response.Body.String(), `"remaining":"3"`)
			response = developerAPIRequest(engine, "/v1/key/balance", "Bearer phase2key2")
			assert.Contains(t, response.Body.String(), `"remaining":"0"`)
			response = developerAPIRequest(engine, "/v1/models", "Bearer phase2key2")
			assert.Equal(t, http.StatusUnauthorized, response.Code, "existing inference auth still denies exhausted quota")
			// The source connection is untrusted, so forwarding headers cannot bypass the key's CIDR restriction.
			request := httptest.NewRequest(http.MethodGet, "/v1/key/balance", nil)
			request.RemoteAddr = "198.51.100.20:1234"
			request.Header.Set("Authorization", "Bearer phase2key10")
			request.Header.Set("X-Forwarded-For", "203.0.113.10")
			response = httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			assert.Equal(t, http.StatusForbidden, response.Code)
			// Personal access tokens and dashboard JWTs are distinct from relay keys.
			pat := "phase2dashboardpat"
			require.NoError(t, model.DB.Model(&user).Update("access_token", pat).Error)
			dashboardJWT, _, err := service.IssueAccessToken(service.AuthIdentity{UserID: user.Id, SessionID: "phase2-session", UserAuthVersion: 1, SessionVersion: 1})
			require.NoError(t, err)
			for _, credential := range []string{pat, dashboardJWT} {
				response = developerAPIRequest(engine, "/v1/key/balance", "Bearer "+credential)
				assert.Equal(t, http.StatusUnauthorized, response.Code)
			}
			for _, test := range []struct {
				display, symbol, unit, remaining string
				rate                             float64
			}{
				{display: "USD", unit: "USD", remaining: "1.5", rate: 1},
				{display: "CNY", unit: "CNY", remaining: "10.5", rate: 7},
				{display: "CUSTOM", symbol: "Điểm", unit: "points", remaining: "0.45", rate: 0.3},
				{display: "CUSTOM", symbol: "Credits", unit: "custom", remaining: "4.5", rate: 3},
			} {
				t.Run("currency "+test.unit+test.symbol, func(t *testing.T) {
					currency := operation_setting.GetGeneralSetting()
					previousCurrency, previousUSD := *currency, operation_setting.USDExchangeRate
					currency.QuotaDisplayType, currency.CustomCurrencySymbol, currency.CustomCurrencyExchangeRate = test.display, test.symbol, test.rate
					operation_setting.USDExchangeRate = test.rate
					defer func() { *currency = previousCurrency; operation_setting.USDExchangeRate = previousUSD }()
					response := developerAPIRequest(engine, "/v1/key/balance", "Bearer phase2key0")
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					var body map[string]any
					require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
					assert.Equal(t, test.unit, body["unit"])
					assert.Equal(t, test.remaining, body["remaining"])
					if test.unit == "custom" {
						assert.Equal(t, test.symbol, body["unit_symbol"])
					}
				})
			}
			currency := operation_setting.GetGeneralSetting()
			previousCurrency := *currency
			currency.QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
			response = developerAPIRequest(engine, "/v1/key/balance", "Bearer phase2key0")
			assert.Equal(t, http.StatusInternalServerError, response.Code)
			assert.NotContains(t, response.Body.String(), "750000")
			*currency = previousCurrency
			currency.CustomCurrencyExchangeRate = math.NaN()
			response = developerAPIRequest(engine, "/v1/key/balance", "Bearer phase2key0")
			assert.Equal(t, http.StatusInternalServerError, response.Code)
			assert.Contains(t, response.Body.String(), `"error"`)
			*currency = previousCurrency
			previousQuotaPerUnit := common.QuotaPerUnit
			for _, value := range []float64{0, math.NaN(), math.Inf(1)} {
				common.QuotaPerUnit = value
				for _, path := range []string{"/v1/catalog", "/v1/key/balance"} {
					response = developerAPIRequest(engine, path, "Bearer phase2key0")
					assert.Equal(t, http.StatusInternalServerError, response.Code)
					assert.NotContains(t, response.Body.String(), "750000")
				}
			}
			common.QuotaPerUnit = previousQuotaPerUnit
			removed := model.Token{UserId: user.Id, Key: "phase2removedkey", Status: 1, ExpiredTime: -1, UnlimitedQuota: true}
			require.NoError(t, model.DB.Create(&removed).Error)
			require.NoError(t, model.DB.Delete(&removed).Error)
			for _, path := range []string{"/v1/catalog", "/v1/key/balance"} {
				response = developerAPIRequest(engine, path, "Bearer phase2removedkey")
				assert.Equal(t, http.StatusUnauthorized, response.Code)
			}
			// Missing owners fail closed rather than permitting a still-present key.
			require.NoError(t, model.DB.Delete(&user).Error)
			for _, path := range []string{"/v1/catalog", "/v1/key/balance"} {
				response = developerAPIRequest(engine, path, "Bearer phase2key0")
				assert.Equal(t, http.StatusInternalServerError, response.Code)
				assert.Contains(t, response.Body.String(), `"error"`)
			}
			var logs int64
			require.NoError(t, model.DB.Model(&model.Log{}).Count(&logs).Error)
			assert.Zero(t, logs, "control-plane reads do not write billing logs")
		})
	}
}

func TestDeveloperAPIReadGateAndRateLimit(t *testing.T) {
	setupDeveloperAPITestDB(t, "sqlite")
	engine := gin.New()
	engine.GET("/v1/unsafe", middleware.PublicAPIKeyReadAuth(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	engine.POST("/v1/catalog", middleware.PublicAPIKeyReadAuth(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	for _, test := range []struct{ method, path string }{{"GET", "/v1/unsafe"}, {"POST", "/v1/catalog"}} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
		assert.Equal(t, http.StatusForbidden, response.Code)
	}
	oldEnabled, oldNum, oldDuration := common.GlobalApiRateLimitEnable, common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration
	common.GlobalApiRateLimitEnable, common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration = true, 1, 60
	defer func() {
		common.GlobalApiRateLimitEnable, common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration = oldEnabled, oldNum, oldDuration
	}()
	rateEngine := gin.New()
	SetRelayRouter(rateEngine)
	response := developerAPIRequest(rateEngine, "/v1/catalog", "")
	assert.Equal(t, http.StatusUnauthorized, response.Code)
	response = developerAPIRequest(rateEngine, "/v1/key/balance", "")
	assert.Equal(t, http.StatusTooManyRequests, response.Code)
	assert.Equal(t, "60", response.Header().Get("Retry-After"))
	assert.Empty(t, response.Body.String(), "existing general API limiter has an empty rejection response")
}

func TestDeveloperCatalogDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			setupDeveloperAPITestDB(t, dialect)
			oldRatio, oldPrice, oldCompletion, oldCache := ratio_setting.ModelRatio2JSONString(), ratio_setting.ModelPrice2JSONString(), ratio_setting.CompletionRatio2JSONString(), ratio_setting.CacheRatio2JSONString()
			billing := config.GlobalConfig.Get("billing_setting").(*billing_setting.BillingSetting)
			oldBilling := *billing
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"phase2-text":1,"phase2-cache-free":1,"phase2-missing":1,"phase2-disabled":1,"phase2-hidden":1,"phase2-opaque-audio":1}`))
			require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"phase2-text":2,"phase2-cache-free":2,"phase2-missing":2}`))
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"phase2-image":0}`))
			require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(`{"phase2-cache-free":0}`))
			billing.BillingMode = map[string]string{"phase2-tier": "tiered_expr", "phase2-variable": "tiered_expr"}
			billing.BillingExpr = map[string]string{"phase2-tier": `tier("base",p*2+c*3+cr*0)`, "phase2-variable": `len<1000? tier("short",p*2+c*3):tier("long",p*4+c*6)`}
			billing.PluginBillingExpr = map[string]string{}
			t.Cleanup(func() {
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio))
				require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrice))
				require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(oldCompletion))
				require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(oldCache))
				*billing = oldBilling
			})
			user := model.User{Username: "catalog-owner", Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, Quota: 0}
			require.NoError(t, model.DB.Create(&user).Error)
			token := model.Token{UserId: user.Id, Key: "phase2catalogkey", Status: common.TokenStatusExhausted, ExpiredTime: -1, RemainQuota: 0, UsedQuota: 123, AccessedTime: 456}
			require.NoError(t, model.DB.Create(&token).Error)
			vendor := model.Vendor{Name: "OpenAI", Description: "private vendor description", Status: 1}
			require.NoError(t, model.DB.Create(&vendor).Error)
			for _, name := range []string{"phase2-text", "phase2-cache-free", "phase2-missing", "phase2-image", "phase2-tier", "phase2-variable", "phase2-disabled", "phase2-hidden", "phase2-unpriced", "phase2-opaque-audio"} {
				channel := model.Channel{Name: "private channel " + name, Key: "upstream-secret", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Group: "default", Models: name}
				if name == "phase2-image" {
					channel.Type = constant.ChannelTypeAdvancedCustom
					channel.SetOtherSettings(relaydto.ChannelOtherSettings{AdvancedCustom: &relaydto.AdvancedCustomConfig{Routes: []relaydto.AdvancedCustomRoute{{IncomingPath: "/v1/images/generations", UpstreamPath: "/v1/images/generations"}}}})
				}
				if name == "phase2-disabled" {
					channel.Status = common.ChannelStatusManuallyDisabled
				}
				require.NoError(t, model.DB.Create(&channel).Error)
				for _, group := range []string{"default", "vip"} {
					require.NoError(t, model.DB.Create(&model.Ability{Group: group, Model: name, ChannelId: channel.Id, Enabled: true}).Error)
				}
				meta := model.Model{ModelName: name, VendorID: vendor.Id, Status: 1}
				if name == "phase2-hidden" {
					meta.Status = 2
				}
				if name == "phase2-opaque-audio" {
					meta.Endpoints = `["audio-speech"]`
				}
				if name == "phase2-image" {
					meta.Endpoints = `["image-generation"]`
				}
				require.NoError(t, model.DB.Create(&meta).Error)
			}
			engine := gin.New()
			engine.Use(middleware.RequestId())
			SetRelayRouter(engine)
			response := developerAPIRequest(engine, "/v1/catalog", "Bearer phase2catalogkey")
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var catalog dto.DeveloperCatalog
			catalog = dto.DeveloperCatalog{}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
			assert.Equal(t, "catalog", catalog.Object)
			assert.Equal(t, "1.0", catalog.SchemaVersion)
			assert.Equal(t, "points", catalog.Currency)
			byID := map[string]dto.DeveloperCatalogModel{}
			ids := make([]string, 0, len(catalog.Models))
			for _, entry := range catalog.Models {
				byID[entry.ID] = entry
				ids = append(ids, entry.ID)
			}
			assert.Equal(t, []string{"phase2-cache-free", "phase2-image", "phase2-missing", "phase2-opaque-audio", "phase2-text", "phase2-tier", "phase2-variable"}, ids)
			require.NotNil(t, byID["phase2-text"].Pricing.InputPer1MTokens)
			assert.Equal(t, "4", *byID["phase2-text"].Pricing.InputPer1MTokens)
			require.NotNil(t, byID["phase2-text"].Pricing.OutputPer1MTokens)
			assert.Equal(t, "8", *byID["phase2-text"].Pricing.OutputPer1MTokens)
			require.NotNil(t, byID["phase2-cache-free"].Pricing.CachedInputPer1MTokens)
			assert.Equal(t, "0", *byID["phase2-cache-free"].Pricing.CachedInputPer1MTokens)
			assert.Nil(t, byID["phase2-missing"].Pricing.CachedInputPer1MTokens)
			require.NotNil(t, byID["phase2-image"].Pricing.PerImage)
			assert.Equal(t, "0", *byID["phase2-image"].Pricing.PerImage)
			require.NotNil(t, byID["phase2-tier"].Pricing.InputPer1MTokens)
			assert.Equal(t, "4", *byID["phase2-tier"].Pricing.InputPer1MTokens)
			require.NotNil(t, byID["phase2-tier"].Pricing.OutputPer1MTokens)
			assert.Equal(t, "6", *byID["phase2-tier"].Pricing.OutputPer1MTokens)
			require.NotNil(t, byID["phase2-tier"].Pricing.CachedInputPer1MTokens)
			assert.Equal(t, "0", *byID["phase2-tier"].Pricing.CachedInputPer1MTokens)
			assert.True(t, byID["phase2-variable"].Pricing.Variable)
			assert.Nil(t, byID["phase2-variable"].Pricing.InputPer1MTokens)
			assert.Nil(t, byID["phase2-variable"].Pricing.OutputPer1MTokens)
			assert.Equal(t, "OpenAI", byID["phase2-text"].Provider)
			assert.Empty(t, byID["phase2-opaque-audio"].Category, "generic native routing is insufficient to classify opaque audio as chat")
			for _, endpoint := range byID["phase2-opaque-audio"].Endpoints {
				assert.NotEqual(t, "/v1/audio/speech", endpoint.Path, "metadata must not manufacture unsupported endpoint capability")
			}
			for _, secret := range []string{"upstream-secret", "private channel", "private vendor", "model_ratio", "completion_ratio", "group_ratio", "billing_expr", "channel_id", "vendor_id", "capabilities", "tier("} {
				assert.NotContains(t, response.Body.String(), secret)
			}
			again := developerAPIRequest(engine, "/v1/catalog", "Bearer phase2catalogkey")
			assert.Equal(t, response.Body.String(), again.Body.String(), "public schema ordering is deterministic")
			require.NoError(t, model.DB.Model(&token).Updates(map[string]any{"model_limits_enabled": true, "model_limits": "phase2-text"}).Error)
			response = developerAPIRequest(engine, "/v1/catalog", "Bearer phase2catalogkey")
			catalog = dto.DeveloperCatalog{}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
			require.Len(t, catalog.Models, 1)
			assert.Equal(t, "phase2-text", catalog.Models[0].ID)
			require.NoError(t, model.DB.Model(&token).Update("group", "vip").Error)
			response = developerAPIRequest(engine, "/v1/catalog", "Bearer phase2catalogkey")
			catalog = dto.DeveloperCatalog{}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
			require.Len(t, catalog.Models, 1)
			assert.Equal(t, "2", *catalog.Models[0].Pricing.InputPer1MTokens)
			assert.Equal(t, "4", *catalog.Models[0].Pricing.OutputPer1MTokens)

			previousGroupOverrides := ratio_setting.GroupGroupRatio2JSONString()
			require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"default":{"vip":0.25}}`))
			response = developerAPIRequest(engine, "/v1/catalog", "Bearer phase2catalogkey")
			catalog = dto.DeveloperCatalog{}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
			require.Len(t, catalog.Models, 1)
			assert.Equal(t, "1", *catalog.Models[0].Pricing.InputPer1MTokens)
			require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(previousGroupOverrides))
			require.NoError(t, model.DB.Model(&token).Updates(map[string]any{"group": "auto", "auto_groups": `["default","vip"]`}).Error)
			response = developerAPIRequest(engine, "/v1/catalog", "Bearer phase2catalogkey")
			catalog = dto.DeveloperCatalog{}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
			require.Len(t, catalog.Models, 1)
			assert.True(t, catalog.Models[0].Pricing.Variable, "different reachable Auto groups cannot have a single advertised price")
			assert.Nil(t, catalog.Models[0].Pricing.InputPer1MTokens)
			require.NoError(t, model.DB.Model(&token).Update("auto_groups", `{"broken":true}`).Error)
			response = developerAPIRequest(engine, "/v1/catalog", "Bearer phase2catalogkey")
			catalog = dto.DeveloperCatalog{}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
			assert.Empty(t, catalog.Models, "malformed custom Auto selection must not inherit all globally available groups")
			var after model.Token
			require.NoError(t, model.DB.First(&after, token.Id).Error)
			assert.Equal(t, 123, after.UsedQuota)
			assert.Zero(t, after.RemainQuota)
			assert.Equal(t, int64(456), after.AccessedTime)
			assert.Equal(t, common.TokenStatusExhausted, after.Status)
		})
	}
}

func TestDeveloperCatalogTaskUsageAndGroupIsolation(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			setupDeveloperAPITestDB(t, dialect)
			previousRegistry := jsplugin.DefaultRegistry
			jsplugin.DefaultRegistry = jsplugin.NewRegistry()
			t.Cleanup(func() { jsplugin.DefaultRegistry = previousRegistry })
			source := `
export const meta={apiVersion:1,key:"phase2-media",name:"Phase 2 Media",version:"1.0.0",author:{name:"Test"},fetchMode:"per_task",
 models:["phase2-video","phase2-audio","phase2-media-tokens","phase2-media-complex"],
 protocols:[{name:"openai_video",models:["phase2-video","phase2-media-complex"]},{name:"openai_audio_speech",models:["phase2-audio","phase2-media-tokens"]}],
 usageProfiles:[
 {models:["phase2-video","phase2-media-complex"],schema:{seconds:{type:"number",unit:"second",description:{en:"Video generation unit price",zh:"视频生成单价"}}}},
 {models:["phase2-audio"],schema:{characters:{type:"number",unit:"character",description:{en:"Speech generation unit price",zh:"语音生成单价"}}}},
 {models:["phase2-media-tokens"],schema:{tokens:{type:"number",unit:"token",description:{en:"Audio generation unit price",zh:"音频生成单价"}}},examples:[{label:"1M audio tokens",facts:{tokens:1000000}}]}
 ]};
export const protocols={openai_video:{decodeRequest:function(){throw new Error("No inference in catalog tests")},render:function(ctx,task){return task}},openai_audio_speech:{decodeRequest:function(){throw new Error("No inference in catalog tests")}}};
export function listArtifacts(){return []}
export function buildContentRequest(){throw new Error("No upstream requests in catalog tests")}
export function buildSubmitRequest(){throw new Error("No upstream requests in catalog tests")}
export function parseSubmitResponse(){return {}}
export function buildQueryRequest(){throw new Error("No upstream requests in catalog tests")}
export function parseTaskResult(){return {}}
`
			_, err := jsplugin.DefaultRegistry.Register(source, jsplugin.Options{})
			require.NoError(t, err)
			billing := config.GlobalConfig.Get("billing_setting").(*billing_setting.BillingSetting)
			previousBilling := *billing
			billing.BillingMode = map[string]string{"phase2-video": "tiered_expr", "phase2-audio": "tiered_expr", "phase2-media-tokens": "tiered_expr", "phase2-media-complex": "tiered_expr"}
			billing.BillingExpr = map[string]string{"phase2-video": `tier("base",u("seconds")*0.4)`, "phase2-audio": `tier("base",u("characters")*0.001)`, "phase2-media-tokens": `tier("base",u("tokens")*9.8/1000000)`, "phase2-media-complex": `u("seconds")>5? tier("long",u("seconds")*0.8):tier("short",u("seconds")*0.4)`}
			billing.PluginBillingExpr = map[string]string{}
			previousRatio := ratio_setting.ModelRatio2JSONString()
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"phase2-shared":1}`))
			t.Cleanup(func() {
				*billing = previousBilling
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousRatio))
			})
			user := model.User{Username: "phase2-media-owner", Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
			require.NoError(t, model.DB.Create(&user).Error)
			token := model.Token{UserId: user.Id, Key: "phase2mediakey", Status: 1, ExpiredTime: -1, UnlimitedQuota: true}
			require.NoError(t, model.DB.Create(&token).Error)
			for _, name := range []string{"phase2-video", "phase2-audio", "phase2-media-tokens", "phase2-media-complex"} {
				channel := model.Channel{Name: "private media", Key: "media-provider-secret", Type: constant.ChannelTypeTaskPlugin, Status: 1, Group: "default", Models: name}
				channel.SetSetting(relaydto.ChannelSettings{TaskPluginKey: "phase2-media"})
				require.NoError(t, model.DB.Create(&channel).Error)
				require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: name, ChannelId: channel.Id, Enabled: true}).Error)
			}
			shared := model.Channel{Name: "ordinary shared", Key: "private-text-secret", Type: constant.ChannelTypeAdvancedCustom, Status: 1, Group: "default", Models: "phase2-shared"}
			shared.SetOtherSettings(relaydto.ChannelOtherSettings{AdvancedCustom: &relaydto.AdvancedCustomConfig{Routes: []relaydto.AdvancedCustomRoute{{IncomingPath: "/v1/chat/completions", UpstreamPath: "/chat"}}}})
			require.NoError(t, model.DB.Create(&shared).Error)
			require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "phase2-shared", ChannelId: shared.Id, Enabled: true}).Error)
			imageSource := `
export const meta={apiVersion:1,key:"phase2-private-image",name:"Private Image",version:"1.0.0",author:{name:"Test"},models:["phase2-shared"],protocols:["openai_image"],fetchMode:"per_task",usageSchema:{image_count:{type:"number",unit:"count",description:{en:"Image generation unit price",zh:"图片生成单价"}}}};
export const protocols={openai_image:{decodeRequest:function(){throw new Error("No inference")},render:function(ctx,task){return task}}};
export function buildSubmitRequest(){throw new Error("No upstream")}
export function parseSubmitResponse(){return {}}
export function buildQueryRequest(){throw new Error("No upstream")}
export function parseTaskResult(){return {}}
`
			_, err = jsplugin.DefaultRegistry.Register(imageSource, jsplugin.Options{})
			require.NoError(t, err)
			billing.PluginBillingExpr["phase2-private-image::phase2-shared"] = `tier("base",u("image_count")*3)`
			private := model.Channel{Name: "private image", Key: "private-image-secret", Type: constant.ChannelTypeTaskPlugin, Status: 1, Group: "vip", Models: "phase2-shared"}
			private.SetSetting(relaydto.ChannelSettings{TaskPluginKey: "phase2-private-image"})
			require.NoError(t, model.DB.Create(&private).Error)
			require.NoError(t, model.DB.Create(&model.Ability{Group: "vip", Model: "phase2-shared", ChannelId: private.Id, Enabled: true}).Error)
			engine := gin.New()
			SetRelayRouter(engine)
			response := developerAPIRequest(engine, "/v1/catalog", "Bearer phase2mediakey")
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var catalog dto.DeveloperCatalog
			catalog = dto.DeveloperCatalog{}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
			byID := map[string]dto.DeveloperCatalogModel{}
			for _, entry := range catalog.Models {
				byID[entry.ID] = entry
			}
			require.Len(t, byID, 5)
			assert.Equal(t, "video", byID["phase2-video"].Category)
			require.NotNil(t, byID["phase2-video"].Pricing.PerSecond)
			assert.Equal(t, "0.8", *byID["phase2-video"].Pricing.PerSecond)
			assert.Equal(t, "audio", byID["phase2-audio"].Category)
			require.NotNil(t, byID["phase2-audio"].Pricing.PerCharacter)
			assert.Equal(t, "0.002", *byID["phase2-audio"].Pricing.PerCharacter)
			assert.Equal(t, []dto.DeveloperCatalogPriceComponent{{Subject: "tokens", Unit: "token_per_1m", Price: "19.6"}}, byID["phase2-media-tokens"].Pricing.Components)
			assert.True(t, byID["phase2-media-complex"].Pricing.Variable)
			assert.Nil(t, byID["phase2-media-complex"].Pricing.PerSecond)
			assert.Equal(t, []dto.DeveloperCatalogEndpoint{{Path: "/v1/chat/completions", Method: "POST"}}, byID["phase2-shared"].Endpoints)
			assert.Nil(t, byID["phase2-shared"].Pricing.PerImage)
			for _, secret := range []string{"media-provider-secret", "private-image-secret", "private-text-secret", "phase2-private-image", "channel_id", "u(\"", "usage_schema"} {
				assert.NotContains(t, response.Body.String(), secret)
			}
			// A provider override may differ even when both executing channels serve the same model.
			otherSource := strings.ReplaceAll(source, "phase2-media", "phase2-alt-media")
			_, err = jsplugin.DefaultRegistry.Register(otherSource, jsplugin.Options{})
			require.NoError(t, err)
			billing.PluginBillingExpr["phase2-alt-media::phase2-video"] = `tier("base",u("seconds")*0.7)`
			alt := model.Channel{Name: "alternate media", Key: "alternate-secret", Type: constant.ChannelTypeTaskPlugin, Status: 1, Group: "default", Models: "phase2-video"}
			alt.SetSetting(relaydto.ChannelSettings{TaskPluginKey: "phase2-alt-media"})
			require.NoError(t, model.DB.Create(&alt).Error)
			require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "phase2-video", ChannelId: alt.Id, Enabled: true}).Error)
			response = developerAPIRequest(engine, "/v1/catalog", "Bearer phase2mediakey")
			catalog = dto.DeveloperCatalog{}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
			for _, entry := range catalog.Models {
				if entry.ID == "phase2-video" {
					assert.True(t, entry.Pricing.Variable)
					assert.Nil(t, entry.Pricing.PerSecond)
				}
			}
		})
	}

}

type developerQueryRecorder struct {
	logger.Interface
	reads, writes int
}

func (recorder *developerQueryRecorder) Trace(ctx context.Context, begin time.Time, sql func() (string, int64), err error) {
	statement, _ := sql()
	prefix := strings.ToUpper(strings.TrimSpace(statement))
	if strings.HasPrefix(prefix, "SELECT") {
		recorder.reads++
	}
	if strings.HasPrefix(prefix, "INSERT") || strings.HasPrefix(prefix, "UPDATE") || strings.HasPrefix(prefix, "DELETE") {
		recorder.writes++
	}
}

func TestDeveloperCatalogReadOnlyAndBatchedQueries(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			setupDeveloperAPITestDB(t, dialect)
			user := model.User{Username: "phase2-batch-owner", Status: 1, Group: "default", AuthVersion: 1}
			require.NoError(t, model.DB.Create(&user).Error)
			token := model.Token{UserId: user.Id, Key: "phase2batchkey", Status: 1, ExpiredTime: -1, UnlimitedQuota: true}
			require.NoError(t, model.DB.Create(&token).Error)
			previousMode := operation_setting.SelfUseModeEnabled
			operation_setting.SelfUseModeEnabled = false
			previousRatio := ratio_setting.ModelRatio2JSONString()
			previousPrice := ratio_setting.ModelPrice2JSONString()
			billing := config.GlobalConfig.Get("billing_setting").(*billing_setting.BillingSetting)
			previousBilling := *billing
			billing.BillingMode = map[string]string{}
			billing.BillingExpr = map[string]string{}
			billing.PluginBillingExpr = map[string]string{}
			t.Cleanup(func() {
				operation_setting.SelfUseModeEnabled = previousMode
				*billing = previousBilling
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousRatio))
				require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousPrice))
			})
			defaults := ratio_setting.GetDefaultPricingMaps()["ModelRatio"]
			defaultJSON, err := common.Marshal(defaults)
			require.NoError(t, err)
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(defaultJSON)))
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
			defaults["phase2-invalid-expr"] = 1
			defaults["phase2-missing-expr"] = 1
			defaultJSON, err = common.Marshal(defaults)
			require.NoError(t, err)
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(defaultJSON)))
			billing.BillingMode["phase2-invalid-expr"] = "tiered_expr"
			billing.BillingMode["phase2-missing-expr"] = "tiered_expr"
			billing.BillingExpr["phase2-invalid-expr"] = "this is not a valid expression"
			channel := model.Channel{Name: "batch", Key: "batch-private-key", Type: constant.ChannelTypeOpenAI, Status: 1, Group: "default", Models: "phase2-missing-expr"}
			require.NoError(t, model.DB.Create(&channel).Error)
			require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "phase2-missing-expr", ChannelId: channel.Id, Enabled: true}).Error)
			engine := gin.New()
			SetRelayRouter(engine)
			recorder := &developerQueryRecorder{Interface: logger.Default.LogMode(logger.Silent)}
			model.DB = model.DB.Session(&gorm.Session{Logger: recorder})
			response := developerAPIRequest(engine, "/v1/catalog", "Bearer phase2batchkey")
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			smallReads := recorder.reads
			assert.Zero(t, recorder.writes)
			var catalog dto.DeveloperCatalog
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
			assert.Empty(t, catalog.Models, "an explicitly configured expression mode never falls back to residual ratios")
			// The complete built-in ratio catalog exercises the query bound as model count grows.
			names := make([]string, 0, len(defaults)+1)
			abilities := make([]model.Ability, 0, len(defaults)+1)
			for name := range defaults {
				if name == "phase2-missing-expr" {
					continue
				}
				names = append(names, name)
				abilities = append(abilities, model.Ability{Group: "default", Model: name, ChannelId: channel.Id, Enabled: true})
			}
			names = append(names, "phase2-unconfigured")
			abilities = append(abilities, model.Ability{Group: "default", Model: "phase2-unconfigured", ChannelId: channel.Id, Enabled: true})
			require.NoError(t, model.DB.CreateInBatches(abilities, 100).Error)
			require.NoError(t, model.DB.Model(&channel).Update("models", strings.Join(names, ",")).Error)
			brokenJSON := "{invalid"
			malformed := model.Channel{Name: "malformed", Key: "malformed-private-key", Type: constant.ChannelTypeOpenAI, Status: 1, Group: "default", Models: "phase2-missing-expr", Setting: &brokenJSON}
			require.NoError(t, model.DB.Create(&malformed).Error)
			require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "phase2-missing-expr", ChannelId: malformed.Id, Enabled: true}).Error)
			recorder.reads, recorder.writes = 0, 0
			start := time.Now()
			response = developerAPIRequest(engine, "/v1/catalog", "Bearer phase2batchkey")
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			duration := time.Since(start)
			assert.Equal(t, smallReads, recorder.reads, "model count must not introduce per-model database reads")
			assert.Zero(t, recorder.writes, "even malformed settings cannot trigger persistent repairs on catalog reads")
			catalog = dto.DeveloperCatalog{}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
			require.NotEmpty(t, catalog.Models)
			for _, entry := range catalog.Models {
				assert.NotContains(t, []string{"phase2-invalid-expr", "phase2-missing-expr", "phase2-unconfigured"}, entry.ID)
			}
			t.Logf("Built-in catalog: %d enabled model names, %d projected models, %d database reads, %s elapsed", len(abilities)+1, len(catalog.Models), recorder.reads, duration)
			var persisted model.Channel
			require.NoError(t, model.DB.First(&persisted, malformed.Id).Error)
			require.NotNil(t, persisted.Setting)
			assert.Equal(t, brokenJSON, *persisted.Setting)
			// Self-use mode follows the existing unconfigured-price permission, without manufacturing an explicit cache price.
			operation_setting.SelfUseModeEnabled = true
			response = developerAPIRequest(engine, "/v1/catalog", "Bearer phase2batchkey")
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			catalog = dto.DeveloperCatalog{}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &catalog))
			foundUnconfigured := false
			for _, entry := range catalog.Models {
				if entry.ID == "phase2-unconfigured" {
					foundUnconfigured = true
					assert.Nil(t, entry.Pricing.CachedInputPer1MTokens)
				}
			}
			assert.True(t, foundUnconfigured)
		})
	}

}

func TestDeveloperKeyBalanceUsesCurrentQuotaCacheAndRevocation(t *testing.T) {
	setupDeveloperAPITestDB(t, "sqlite")
	redisServer := miniredis.RunT(t)
	previousRDB, previousRedis := common.RDB, common.RedisEnabled
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	common.RDB, common.RedisEnabled = client, true
	t.Cleanup(func() {
		common.RDB, common.RedisEnabled = previousRDB, previousRedis
		require.NoError(t, client.Close())
	})
	user := model.User{Username: "phase2-cache-owner", Status: 1, Group: "default", AuthVersion: 1, Quota: 0}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "phase2cachedkey", Status: 1, ExpiredTime: -1, RemainQuota: 750000, UsedQuota: 250000, AccessedTime: 123}
	require.NoError(t, model.DB.Create(&token).Error)
	// Hydrate through the production cache reader, then simulate a completed atomic reservation.
	_, err := model.GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	require.NoError(t, client.HSet(context.Background(), "token:"+common.GenerateHMAC(token.Key), "RemainQuota", 500000, "UsedQuota", 500000).Err())
	engine := gin.New()
	SetRelayRouter(engine)
	response := developerAPIRequest(engine, "/v1/key/balance", "Bearer phase2cachedkey")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var body map[string]any
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
	assert.Equal(t, "2", body["remaining"])
	assert.Equal(t, "2", body["used"])
	assert.Equal(t, "4", body["total"])
	var persisted model.Token
	require.NoError(t, model.DB.First(&persisted, token.Id).Error)
	assert.Equal(t, 750000, persisted.RemainQuota)
	assert.Equal(t, 250000, persisted.UsedQuota)
	assert.Equal(t, int64(123), persisted.AccessedTime)
	// The existing mutation fence makes a revoked key unusable despite a preheated cache.
	token.Status = common.TokenStatusDisabled
	require.NoError(t, token.Update())
	response = developerAPIRequest(engine, "/v1/key/balance", "Bearer phase2cachedkey")
	assert.Equal(t, http.StatusUnauthorized, response.Code)
	response = developerAPIRequest(engine, "/v1/catalog", "Bearer phase2cachedkey")
	assert.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestDeveloperReadAuthenticationDoesNotLogBearerKeys(t *testing.T) {
	setupDeveloperAPITestDB(t, "sqlite")
	user := model.User{Username: "phase2-log-owner", Status: 1, Group: "default", AuthVersion: 1}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "phase2-log-secret-key", Status: 1, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, model.DB.Create(&token).Error)
	var sqlLog bytes.Buffer
	queryLogger := logger.New(log.New(&sqlLog, "", 0), logger.Config{LogLevel: logger.Info})
	model.DB = model.DB.Session(&gorm.Session{Logger: queryLogger})
	// Calibrate against the existing legacy query, which keeps its logging behavior.
	_, err := model.GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	require.Contains(t, sqlLog.String(), token.Key)
	engine := gin.New()
	SetRelayRouter(engine)
	for _, path := range []string{"/v1/catalog", "/v1/key/balance"} {
		for _, key := range []string{token.Key, "phase2-invalid-secret-key"} {
			sqlLog.Reset()
			response := developerAPIRequest(engine, path, "Bearer "+key)
			if key == token.Key {
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			} else {
				require.Equal(t, http.StatusUnauthorized, response.Code)
			}
			assert.NotContains(t, sqlLog.String(), key, "debug query tracing must not print public bearer credentials")
		}
	}
}
