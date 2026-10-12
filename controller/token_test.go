package controller

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type tokenAPIResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type tokenPageResponse struct {
	Items []tokenResponseItem `json:"items"`
}

type tokenResponseItem struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Key    string `json:"key"`
	Status int    `json:"status"`
}

type tokenKeyResponse struct {
	Key string `json:"key"`
}

type sqliteColumnInfo struct {
	Name string `gorm:"column:name"`
	Type string `gorm:"column:type"`
}

type legacyToken struct {
	Id                 int    `gorm:"primaryKey"`
	UserId             int    `gorm:"index"`
	Key                string `gorm:"column:key;type:char(48);uniqueIndex"`
	Status             int    `gorm:"default:1"`
	Name               string `gorm:"index"`
	CreatedTime        int64  `gorm:"bigint"`
	AccessedTime       int64  `gorm:"bigint"`
	ExpiredTime        int64  `gorm:"bigint;default:-1"`
	RemainQuota        int    `gorm:"default:0"`
	UnlimitedQuota     bool
	ModelLimitsEnabled bool
	ModelLimits        string  `gorm:"type:text"`
	AllowIps           *string `gorm:"default:''"`
	UsedQuota          int     `gorm:"default:0"`
	Group              string  `gorm:"column:group;default:''"`
	CrossGroupRetry    bool
	DeletedAt          gorm.DeletedAt `gorm:"index"`
}

func (legacyToken) TableName() string {
	return "tokens"
}

func openTokenControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	model.DB = db
	model.LOG_DB = db

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func migrateTokenControllerTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()

	if err := db.AutoMigrate(&model.Token{}); err != nil {
		t.Fatalf("failed to migrate token table: %v", err)
	}
}

func setupTokenControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db := openTokenControllerTestDB(t)
	migrateTokenControllerTestDB(t, db)
	return db
}

func openTokenControllerExternalDB(t *testing.T, dialect string, dsn string) (*gorm.DB, *bool) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false

	var (
		db     *gorm.DB
		dbType common.DatabaseType
		err    error
	)
	switch dialect {
	case "mysql":
		dbType = common.DatabaseTypeMySQL
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	case "postgres":
		dbType = common.DatabaseTypePostgreSQL
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	default:
		t.Fatalf("unsupported dialect %q", dialect)
	}
	common.SetDatabaseTypes(dbType, dbType)
	if err != nil {
		t.Fatalf("failed to open %s db: %v", dialect, err)
	}

	model.DB = db
	model.LOG_DB = db

	if db.Migrator().HasTable("tokens") {
		t.Skipf("refusing to run %s migration compatibility test against external database because tokens table already exists", dialect)
	}

	managedTokensTable := new(bool)

	t.Cleanup(func() {
		if *managedTokensTable && db.Migrator().HasTable("tokens") {
			_ = db.Migrator().DropTable("tokens")
		}
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db, managedTokensTable
}

func seedToken(t *testing.T, db *gorm.DB, userID int, name string, rawKey string) *model.Token {
	t.Helper()

	token := &model.Token{
		UserId:         userID,
		Name:           name,
		Key:            rawKey,
		Status:         common.TokenStatusEnabled,
		CreatedTime:    1,
		AccessedTime:   1,
		ExpiredTime:    -1,
		RemainQuota:    100,
		UnlimitedQuota: true,
		Group:          "default",
	}
	if err := db.Create(token).Error; err != nil {
		t.Fatalf("failed to create token: %v", err)
	}
	return token
}

func newAuthenticatedContext(t *testing.T, method string, target string, body any, userID int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	var requestBody *bytes.Reader
	if body != nil {
		payload, err := common.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}
		requestBody = bytes.NewReader(payload)
	} else {
		requestBody = bytes.NewReader(nil)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, requestBody)
	if body != nil {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	ctx.Set("id", userID)
	return ctx, recorder
}

func decodeAPIResponse(t *testing.T, recorder *httptest.ResponseRecorder) tokenAPIResponse {
	t.Helper()

	var response tokenAPIResponse
	if err := common.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode api response: %v", err)
	}
	return response
}

func getSQLiteColumnType(t *testing.T, db *gorm.DB, tableName string, columnName string) string {
	t.Helper()

	var columns []sqliteColumnInfo
	if err := db.Raw("PRAGMA table_info(" + tableName + ")").Scan(&columns).Error; err != nil {
		t.Fatalf("failed to inspect %s schema: %v", tableName, err)
	}

	for _, column := range columns {
		if column.Name == columnName {
			return strings.ToLower(column.Type)
		}
	}

	t.Fatalf("column %s not found in %s schema", columnName, tableName)
	return ""
}

func getTokenKeyColumnType(t *testing.T, db *gorm.DB, dialect string) string {
	t.Helper()

	switch dialect {
	case "sqlite":
		return getSQLiteColumnType(t, db, "tokens", "key")
	case "mysql":
		var columnType string
		if err := db.Raw(`SELECT COLUMN_TYPE FROM information_schema.columns
			WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
			"tokens", "key").Scan(&columnType).Error; err != nil {
			t.Fatalf("failed to inspect mysql token key column: %v", err)
		}
		return strings.ToLower(columnType)
	case "postgres":
		var dataType string
		var maxLength sql.NullInt64
		if err := db.Raw(`SELECT data_type, character_maximum_length
			FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
			"tokens", "key").Row().Scan(&dataType, &maxLength); err != nil {
			t.Fatalf("failed to inspect postgres token key column: %v", err)
		}
		switch strings.ToLower(dataType) {
		case "character varying":
			return fmt.Sprintf("varchar(%d)", maxLength.Int64)
		case "character":
			return fmt.Sprintf("char(%d)", maxLength.Int64)
		default:
			if maxLength.Valid {
				return fmt.Sprintf("%s(%d)", strings.ToLower(dataType), maxLength.Int64)
			}
			return strings.ToLower(dataType)
		}
	default:
		t.Fatalf("unsupported dialect %q", dialect)
		return ""
	}
}

func getTokenAutoGroupsColumnType(t *testing.T, db *gorm.DB, dialect string) string {
	t.Helper()

	switch dialect {
	case "sqlite":
		return getSQLiteColumnType(t, db, "tokens", "auto_groups")
	case "mysql":
		var columnType string
		if err := db.Raw(`SELECT DATA_TYPE FROM information_schema.columns
			WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
			"tokens", "auto_groups").Scan(&columnType).Error; err != nil {
			t.Fatalf("failed to inspect mysql token auto_groups column: %v", err)
		}
		return strings.ToLower(columnType)
	case "postgres":
		var dataType string
		if err := db.Raw(`SELECT data_type FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
			"tokens", "auto_groups").Scan(&dataType).Error; err != nil {
			t.Fatalf("failed to inspect postgres token auto_groups column: %v", err)
		}
		return strings.ToLower(dataType)
	default:
		t.Fatalf("unsupported dialect %q", dialect)
		return ""
	}
}

func runTokenMigrationCompatibilityTest(t *testing.T, db *gorm.DB, dialect string, managedTokensTable *bool) {
	t.Helper()

	legacyKey := strings.Repeat("a", 48)
	longKey := strings.Repeat("b", 64)

	if err := db.AutoMigrate(&legacyToken{}); err != nil {
		t.Fatalf("failed to create legacy token schema: %v", err)
	}
	if managedTokensTable != nil {
		*managedTokensTable = true
	}
	if err := db.Create(&legacyToken{
		UserId:             7,
		Key:                legacyKey,
		Status:             common.TokenStatusEnabled,
		Name:               "legacy-token",
		CreatedTime:        1,
		AccessedTime:       1,
		ExpiredTime:        -1,
		RemainQuota:        100,
		UnlimitedQuota:     true,
		ModelLimitsEnabled: false,
		ModelLimits:        "",
		AllowIps:           common.GetPointer(""),
		UsedQuota:          0,
		Group:              "default",
		CrossGroupRetry:    false,
	}).Error; err != nil {
		t.Fatalf("failed to seed legacy token row: %v", err)
	}

	if got := getTokenKeyColumnType(t, db, dialect); got != "char(48)" {
		t.Fatalf("expected legacy key column type char(48), got %q", got)
	}

	migrateTokenControllerTestDB(t, db)

	if got := getTokenKeyColumnType(t, db, dialect); got != "varchar(128)" {
		t.Fatalf("expected migrated key column type varchar(128), got %q", got)
	}
	if !db.Migrator().HasColumn(&model.Token{}, "auto_groups") {
		t.Fatal("expected migration to add auto_groups column")
	}
	if got := getTokenAutoGroupsColumnType(t, db, dialect); got != "text" {
		t.Fatalf("expected migrated auto_groups column type text, got %q", got)
	}

	var migratedToken model.Token
	if err := db.First(&migratedToken, "name = ?", "legacy-token").Error; err != nil {
		t.Fatalf("failed to load migrated token row: %v", err)
	}
	if migratedToken.Key != legacyKey {
		t.Fatalf("expected migrated token key %q, got %q", legacyKey, migratedToken.Key)
	}
	if migratedToken.Name != "legacy-token" {
		t.Fatalf("expected migrated token name to be preserved, got %q", migratedToken.Name)
	}
	if migratedToken.AutoGroups != "" {
		t.Fatalf("expected legacy token to inherit global Auto groups, got %q", migratedToken.AutoGroups)
	}

	inserted := model.Token{
		UserId:             8,
		Name:               "long-token",
		Key:                longKey,
		Status:             common.TokenStatusEnabled,
		CreatedTime:        1,
		AccessedTime:       1,
		ExpiredTime:        -1,
		RemainQuota:        200,
		UnlimitedQuota:     true,
		ModelLimitsEnabled: false,
		ModelLimits:        "",
		AllowIps:           common.GetPointer(""),
		UsedQuota:          0,
		Group:              "default",
		CrossGroupRetry:    false,
	}
	if err := db.Create(&inserted).Error; err != nil {
		t.Fatalf("failed to insert long token after migration: %v", err)
	}

	var fetched model.Token
	if err := db.First(&fetched, "id = ?", inserted.Id).Error; err != nil {
		t.Fatalf("failed to fetch long token after migration: %v", err)
	}
	if fetched.Key != longKey {
		t.Fatalf("expected long token key %q, got %q", longKey, fetched.Key)
	}
}

func TestTokenAutoMigrateUsesVarchar128KeyColumn(t *testing.T) {
	db := setupTokenControllerTestDB(t)

	if got := getTokenKeyColumnType(t, db, "sqlite"); got != "varchar(128)" {
		t.Fatalf("expected key column type varchar(128), got %q", got)
	}
	if got := getSQLiteColumnType(t, db, "tokens", "auto_groups"); got != "text" {
		t.Fatalf("expected auto_groups column type text, got %q", got)
	}
}

func TestTokenMigrationFromChar48ToVarchar128(t *testing.T) {
	db := openTokenControllerTestDB(t)
	runTokenMigrationCompatibilityTest(t, db, "sqlite", nil)
}

func TestTokenMigrationFromChar48ToVarchar128MySQL(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set TEST_MYSQL_DSN to run mysql migration compatibility test")
	}

	db, managedTokensTable := openTokenControllerExternalDB(t, "mysql", dsn)
	runTokenMigrationCompatibilityTest(t, db, "mysql", managedTokensTable)
}

func TestTokenMigrationFromChar48ToVarchar128Postgres(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set TEST_POSTGRES_DSN to run postgres migration compatibility test")
	}

	db, managedTokensTable := openTokenControllerExternalDB(t, "postgres", dsn)
	runTokenMigrationCompatibilityTest(t, db, "postgres", managedTokensTable)
}

func TestGetAllTokensMasksKeyInResponse(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "list-token", "abcd1234efgh5678")
	seedToken(t, db, 2, "other-user-token", "zzzz1234yyyy5678")

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/token/?p=1&size=10", nil, 1)
	GetAllTokens(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}

	var page tokenPageResponse
	if err := common.Unmarshal(response.Data, &page); err != nil {
		t.Fatalf("failed to decode token page response: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected exactly one token, got %d", len(page.Items))
	}
	if page.Items[0].Key != token.GetMaskedKey() {
		t.Fatalf("expected masked key %q, got %q", token.GetMaskedKey(), page.Items[0].Key)
	}
	if strings.Contains(recorder.Body.String(), token.Key) {
		t.Fatalf("list response leaked raw token key: %s", recorder.Body.String())
	}
}

func TestSearchTokensMasksKeyInResponse(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "searchable-token", "ijkl1234mnop5678")

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/token/search?keyword=searchable-token&p=1&size=10", nil, 1)
	SearchTokens(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}

	var page tokenPageResponse
	if err := common.Unmarshal(response.Data, &page); err != nil {
		t.Fatalf("failed to decode search response: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected exactly one search result, got %d", len(page.Items))
	}
	if page.Items[0].Key != token.GetMaskedKey() {
		t.Fatalf("expected masked search key %q, got %q", token.GetMaskedKey(), page.Items[0].Key)
	}
	if strings.Contains(recorder.Body.String(), token.Key) {
		t.Fatalf("search response leaked raw token key: %s", recorder.Body.String())
	}
}

func TestGetTokenMasksKeyInResponse(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "detail-token", "qrst1234uvwx5678")

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/token/"+strconv.Itoa(token.Id), nil, 1)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(token.Id)}}
	GetToken(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}

	var detail tokenResponseItem
	if err := common.Unmarshal(response.Data, &detail); err != nil {
		t.Fatalf("failed to decode token detail response: %v", err)
	}
	if detail.Key != token.GetMaskedKey() {
		t.Fatalf("expected masked detail key %q, got %q", token.GetMaskedKey(), detail.Key)
	}
	if strings.Contains(recorder.Body.String(), token.Key) {
		t.Fatalf("detail response leaked raw token key: %s", recorder.Body.String())
	}
}

func TestUpdateTokenMasksKeyInResponse(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "editable-token", "yzab1234cdef5678")

	body := map[string]any{
		"id":                   token.Id,
		"name":                 "updated-token",
		"expired_time":         -1,
		"remain_quota":         100,
		"unlimited_quota":      true,
		"model_limits_enabled": false,
		"model_limits":         "",
		"group":                "default",
		"cross_group_retry":    false,
	}

	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", body, 1)
	UpdateToken(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}

	var detail tokenResponseItem
	if err := common.Unmarshal(response.Data, &detail); err != nil {
		t.Fatalf("failed to decode token update response: %v", err)
	}
	if detail.Key != token.GetMaskedKey() {
		t.Fatalf("expected masked update key %q, got %q", token.GetMaskedKey(), detail.Key)
	}
	if strings.Contains(recorder.Body.String(), token.Key) {
		t.Fatalf("update response leaked raw token key: %s", recorder.Body.String())
	}
}

func TestGetTokenKeyRequiresOwnershipAndReturnsFullKey(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "owned-token", "owner1234token5678")

	authorizedCtx, authorizedRecorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/"+strconv.Itoa(token.Id)+"/key", nil, 1)
	authorizedCtx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(token.Id)}}
	GetTokenKey(authorizedCtx)

	authorizedResponse := decodeAPIResponse(t, authorizedRecorder)
	if !authorizedResponse.Success {
		t.Fatalf("expected authorized key fetch to succeed, got message: %s", authorizedResponse.Message)
	}

	var keyData tokenKeyResponse
	if err := common.Unmarshal(authorizedResponse.Data, &keyData); err != nil {
		t.Fatalf("failed to decode token key response: %v", err)
	}
	if keyData.Key != token.GetFullKey() {
		t.Fatalf("expected full key %q, got %q", token.GetFullKey(), keyData.Key)
	}

	unauthorizedCtx, unauthorizedRecorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/"+strconv.Itoa(token.Id)+"/key", nil, 2)
	unauthorizedCtx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(token.Id)}}
	GetTokenKey(unauthorizedCtx)

	unauthorizedResponse := decodeAPIResponse(t, unauthorizedRecorder)
	if unauthorizedResponse.Success {
		t.Fatalf("expected unauthorized key fetch to fail")
	}
	if strings.Contains(unauthorizedRecorder.Body.String(), token.Key) {
		t.Fatalf("unauthorized key response leaked raw token key: %s", unauthorizedRecorder.Body.String())
	}
}

func TestAPITokenAuditDatabaseMatrix(t *testing.T) {
	for _, database := range []struct {
		name, env string
		typ       common.DatabaseType
	}{
		{"sqlite", "", common.DatabaseTypeSQLite},
		{"mysql", "AUDIT_MYSQL_DSN", common.DatabaseTypeMySQL},
		{"postgres", "AUDIT_POSTGRES_DSN", common.DatabaseTypePostgreSQL},
	} {
		for _, separateLog := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/separate_log=%v", database.name, separateLog), func(t *testing.T) {
				dsn := os.Getenv(database.env)
				if database.env != "" && dsn == "" {
					t.Skip(database.env + " is not configured")
				}
				previousDB, previousLogDB := model.DB, model.LOG_DB
				previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
				previousRedis, previousMaster, previousSecret := common.RedisEnabled, common.IsMasterNode, common.SessionSecret
				t.Cleanup(func() {
					model.DB, model.LOG_DB = previousDB, previousLogDB
					common.SetDatabaseTypes(previousMain, previousLog)
					common.RedisEnabled, common.IsMasterNode, common.SessionSecret = previousRedis, previousMaster, previousSecret
				})
				common.RedisEnabled, common.IsMasterNode = false, true
				common.SessionSecret = "api-token-audit-test-secret"
				t.Setenv("LOG_SQL_DSN", "")
				db, _ := newAuditTestDatabase(t, database.name, dsn)
				model.DB = db
				common.SetDatabaseTypes(database.typ, database.typ)
				require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.Token{}))
				// Initialize production column quoting as well as the existing audit table.
				require.NoError(t, model.InitLogDB())
				if separateLog {
					logDB, _ := newAuditTestDatabase(t, database.name, dsn)
					model.LOG_DB = logDB
					require.NoError(t, model.MigrateAuditLogs())
				}
				versionSQL := "SELECT version()"
				if database.name == "sqlite" {
					versionSQL = "SELECT sqlite_version()"
				}
				var version string
				require.NoError(t, db.Raw(versionSQL).Scan(&version).Error)
				t.Logf("database version: %s", version)
				verifyAPITokenAudit(t)
				if separateLog {
					var count int64
					require.NoError(t, db.Model(&model.AuditLog{}).Count(&count).Error)
					assert.Zero(t, count, "all audit events must use the configured log database")
				}
			})
		}
	}
}

func verifyAPITokenAudit(t *testing.T) {
	t.Helper()
	pat := "api-token-audit-pat-secret"
	user := &model.User{Username: "token-owner", Password: "placeholder", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AccessToken: &pat, AffCode: "token-owner"}
	require.NoError(t, model.DB.Create(user).Error)
	other := &model.User{Username: "other-owner", Password: "placeholder", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "other-owner"}
	require.NoError(t, model.DB.Create(other).Error)
	session := &model.UserSession{SID: "token-audit-session", UserID: user.Id, Version: 1, UserAuthVersion: 1, Status: model.UserSessionStatusActive, RefreshHash: "placeholder", LoginMethod: "password", LastActiveAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix()}
	require.NoError(t, model.CreateUserSession(session))
	jwt, _, err := service.IssueAccessToken(service.AuthIdentity{UserID: user.Id, SessionID: session.SID, UserAuthVersion: 1, SessionVersion: 1})
	require.NoError(t, err)
	router := gin.New()
	router.Use(middleware.RequestId(), middleware.AccessTokenAudit())
	tokenRoutes := router.Group("/api/token", middleware.UserAuth(), middleware.TokenOperationAudit())
	tokenRoutes.POST("/", AddToken)
	tokenRoutes.PUT("/", UpdateToken)
	tokenRoutes.DELETE("/:id", DeleteToken)
	tokenRoutes.POST("/batch", DeleteTokenBatch)
	tokenRoutes.POST("/batch/keys", GetTokenKeysBatch)
	tokenRoutes.POST("/:id/key", func(c *gin.Context) {
		if c.GetHeader("X-Test-Limit") != "" {
			c.AbortWithStatusJSON(429, gin.H{"success": false})
			return
		}
		c.Next()
	}, GetTokenKey)
	tokenRoutes.GET("/", GetAllTokens)
	tokenRoutes.GET("/:id", GetToken)
	tokenRoutes.GET("/search", SearchTokens)
	router.GET("/api/audit/self", middleware.UserAuth(), GetAuditLogs)

	for index, tc := range []struct {
		name, method, path, body, action string
		success                          bool
		params                           string
		initialStatus                    int
		failWrite, rateLimit, usePAT     bool
	}{
		{name: "create", method: "POST", path: "/", body: `{"name":"created","expired_time":-1,"unlimited_quota":true}`, action: "token.create", success: true},
		{name: "invalid create", method: "POST", path: "/", body: `{"name":"attempt","remain_quota":-1}`, action: "token.create", params: `{"name":"attempt"}`},
		{name: "malformed body", method: "POST", path: "/", body: `{"key":"raw-body-secret"`, action: "token.create", params: `{}`},
		{name: "validation error exceeds audit buffer", method: "POST", path: "/", body: `{"remain_quota":` + strings.Repeat("9", 64*1024) + `}`, action: "token.create", params: `{}`},
		{name: "create storage failure", method: "POST", path: "/", body: `{"name":"attempt","unlimited_quota":true}`, action: "token.create", params: `{"name":"attempt"}`, failWrite: true},
		{name: "normalized update", method: "PUT", path: "/", body: `{"id":$id,"name":"renamed","expired_time":-1,"remain_quota":200,"unlimited_quota":true,"group":"default","cross_group_retry":true,"allow_ips":""}`, action: "token.update", success: true, params: `{"id":$id,"name":"renamed","changed_fields":["name","remain_quota","group","cross_group_retry","auto_groups"]}`},
		{name: "configuration values stay private", method: "PUT", path: "/", body: `{"id":$id,"name":"owned","expired_time":42,"remain_quota":100,"unlimited_quota":false,"model_limits_enabled":true,"model_limits":"private-model-configuration","allow_ips":"203.0.113.57","group":"auto","cross_group_retry":true}`, action: "token.update", success: true, params: `{"id":$id,"name":"owned","changed_fields":["expired_time","unlimited_quota","model_limits_enabled","model_limits","allow_ips"]}`},
		{name: "unchanged update", method: "PUT", path: "/", body: `{"id":$id,"name":"owned","expired_time":-1,"remain_quota":100,"unlimited_quota":true,"group":"auto","cross_group_retry":true,"allow_ips":""}`, action: "token.update", success: true, params: `{"id":$id,"name":"owned","changed_fields":[]}`},
		{name: "successful response exceeds audit buffer", method: "PUT", path: "/", body: `{"id":$id,"name":"owned","expired_time":-1,"remain_quota":100,"unlimited_quota":true,"group":"auto","cross_group_retry":true,"allow_ips":"","model_limits":"` + strings.Repeat("m", 64*1024-128) + `"}`, action: "token.update", success: true, params: `{"id":$id,"name":"owned","changed_fields":["model_limits"]}`},
		{name: "update storage failure", method: "PUT", path: "/", body: `{"id":$id,"name":"failed-rename","unlimited_quota":true}`, action: "token.update", params: `{"id":$id,"name":"owned"}`, failWrite: true},
		{name: "foreign update", method: "PUT", path: "/", body: `{"id":$other,"name":"forged-name","unlimited_quota":true}`, action: "token.update", params: `{"id":$other}`},
		{name: "disable", method: "PUT", path: "/?status_only=true", body: `{"id":$id,"status":2}`, action: "token.status_update", success: true, params: `{"id":$id,"name":"owned","from":1,"to":2}`},
		{name: "enable", method: "PUT", path: "/?status_only=true", body: `{"id":$id,"status":1}`, action: "token.status_update", success: true, params: `{"id":$id,"name":"owned","from":2,"to":1}`, initialStatus: common.TokenStatusDisabled},
		{name: "expired enable", method: "PUT", path: "/?status_only=true", body: `{"id":$id,"status":1}`, action: "token.status_update", params: `{"id":$id,"name":"owned"}`, initialStatus: common.TokenStatusExpired},
		{name: "delete", method: "DELETE", path: "/$id", action: "token.delete", success: true, params: `{"id":$id,"name":"owned"}`},
		{name: "foreign delete", method: "DELETE", path: "/$other", action: "token.delete", params: `{"id":$other}`},
		{name: "missing delete", method: "DELETE", path: "/999999", action: "token.delete", params: `{"id":999999}`},
		{name: "key view", method: "POST", path: "/$id/key", action: "token.key_view", success: true, params: `{"id":$id,"name":"owned"}`},
		{name: "PAT key view", method: "POST", path: "/$id/key", action: "token.key_view", success: true, params: `{"id":$id,"name":"owned"}`, usePAT: true},
		{name: "foreign key view", method: "POST", path: "/$other/key", action: "token.key_view", params: `{"id":$other}`},
		{name: "rate limited key view", method: "POST", path: "/$id/key", action: "token.key_view", params: `{"id":$id}`, rateLimit: true},
		{name: "batch delete partial and duplicate", method: "POST", path: "/batch", body: `{"ids":[$id,$id,$other,999999]}`, action: "token.delete_batch", success: true, params: `{"requested_ids":[$id,$id,$other,999999],"total":4,"count":1}`},
		{name: "empty batch delete", method: "POST", path: "/batch", body: `{"ids":[]}`, action: "token.delete_batch", params: `{"requested_ids":[],"total":0}`},
		{name: "batch keys partial and duplicate", method: "POST", path: "/batch/keys", body: `{"ids":[$id,$id,$other,999999]}`, action: "token.key_view_batch", success: true, params: `{"requested_ids":[$id,$id,$other,999999],"total":4,"count":1,"returned_ids":[$id]}`},
		{name: "batch keys no matches", method: "POST", path: "/batch/keys", body: `{"ids":[$other,999999]}`, action: "token.key_view_batch", success: true, params: `{"requested_ids":[$other,999999],"total":2,"count":0,"returned_ids":[]}`},
		{name: "empty batch keys", method: "POST", path: "/batch/keys", body: `{"ids":[]}`, action: "token.key_view_batch", params: `{"requested_ids":[],"total":0}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			empty := ""
			owned := &model.Token{UserId: user.Id, Name: "owned", Key: fmt.Sprintf("owned-key-secret-%d", index), Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 100, UnlimitedQuota: true, Group: "auto", CrossGroupRetry: true, AutoGroups: `["default"]`, AllowIps: &empty}
			if tc.initialStatus != 0 {
				owned.Status = tc.initialStatus
			}
			if owned.Status == common.TokenStatusExpired {
				owned.ExpiredTime = 1
			}
			foreign := &model.Token{UserId: other.Id, Name: "private-foreign-name", Key: fmt.Sprintf("foreign-key-secret-%d", index)}
			require.NoError(t, model.DB.Create(owned).Error)
			require.NoError(t, model.DB.Create(foreign).Error)
			replace := strings.NewReplacer("$id", strconv.Itoa(owned.Id), "$other", strconv.Itoa(foreign.Id))
			if tc.failWrite {
				fail := func(tx *gorm.DB) {
					if tx.Statement.Table == "tokens" {
						_ = tx.AddError(errors.New("raw-storage-error-secret"))
					}
				}
				if tc.method == "POST" {
					require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register("token-audit:fail", fail))
					t.Cleanup(func() { require.NoError(t, model.DB.Callback().Create().Remove("token-audit:fail")) })
				} else {
					require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register("token-audit:fail", fail))
					t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove("token-audit:fail")) })
				}
			}
			request := httptest.NewRequest(tc.method, "/api/token"+replace.Replace(tc.path), strings.NewReader(replace.Replace(tc.body)))
			credential := jwt
			if tc.usePAT {
				credential = pat
			}
			request.Header.Set("Authorization", "Bearer "+credential)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("User-Agent", "api-token-audit-client")
			request.RemoteAddr = "192.0.2.12:4321"
			if tc.rateLimit {
				request.Header.Set("X-Test-Limit", "1")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if strings.Contains(tc.name, "exceeds audit buffer") {
				assert.Greater(t, response.Body.Len(), 64*1024)
			}
			var events []model.AuditLog
			require.NoError(t, model.LOG_DB.Where("request_id = ?", response.Header().Get(common.RequestIdKey)).Find(&events).Error)
			expectedEvents := 1
			if tc.usePAT {
				expectedEvents = 2
			}
			require.Len(t, events, expectedEvents)
			var operation *model.AuditLog
			for i := range events {
				if events[i].Category == model.AuditCategorySecurity {
					require.Nil(t, operation, "one operation event per request")
					operation = &events[i]
				} else {
					assert.Equal(t, model.AuditCategoryAccessToken, events[i].Category)
					assert.Equal(t, model.AccessTokenFingerprint(pat), events[i].TokenRef)
				}
			}
			require.NotNil(t, operation)
			assert.Equal(t, tc.action, operation.Action)
			assert.Equal(t, tc.success, operation.Success)
			assert.Equal(t, response.Code, operation.Status)
			if tc.rateLimit {
				assert.Equal(t, 429, response.Code)
			} else {
				assert.Equal(t, 200, response.Code)
				assert.Equal(t, tc.success, decodeAPIResponse(t, response).Success)
			}
			assert.Equal(t, user.Id, operation.UserId)
			assert.Equal(t, user.Username, operation.Username)
			assert.Equal(t, common.RoleCommonUser, operation.ActorRole)
			assert.Equal(t, "192.0.2.12", operation.Ip)
			assert.Equal(t, "api-token-audit-client", operation.UserAgent)
			assert.Equal(t, tc.method, operation.Method)
			expectedRoute := strings.NewReplacer("$id", ":id", "$other", ":id", "999999", ":id").Replace(strings.Split(tc.path, "?")[0])
			assert.Equal(t, "/api/token"+expectedRoute, operation.Route)
			assert.NotEmpty(t, operation.RequestId)
			assert.Empty(t, operation.TokenRef)
			assert.Nil(t, operation.Other.AdminInfo)
			authMethod := "session"
			if tc.usePAT {
				authMethod = "access_token"
			}
			assert.Equal(t, authMethod, operation.AuthMethod)
			require.NotNil(t, operation.Other.Op)
			assert.Equal(t, tc.action, operation.Other.Op.Action)
			params, err := common.Marshal(operation.Other.Op.Params)
			require.NoError(t, err)
			if tc.name == "create" {
				var created model.Token
				require.NoError(t, model.DB.Where("user_id = ? AND name = ?", user.Id, "created").First(&created).Error)
				assert.JSONEq(t, fmt.Sprintf(`{"id":%d,"name":"created"}`, created.Id), string(params))
				assert.NotContains(t, string(params), created.Key)
			} else if tc.params == `{}` {
				assert.Empty(t, operation.Other.Op.Params)
			} else {
				assert.JSONEq(t, replace.Replace(tc.params), string(params))
			}
			encoded, err := common.Marshal(events)
			require.NoError(t, err)
			for _, secret := range []string{pat, jwt, owned.Key, foreign.Key, foreign.Name, "raw-body-secret", "raw-storage-error-secret", "private-model-configuration", "203.0.113.57", "Authorization"} {
				assert.NotContains(t, string(encoded), secret)
			}
			if tc.action == "token.key_view" && tc.success {
				assert.Contains(t, response.Body.String(), owned.GetFullKey())
			}
		})
	}

	t.Run("bounded batch metadata", func(t *testing.T) {
		ids := make([]int, 101)
		for i := range ids {
			ids[i] = 10000 + i
		}
		body, err := common.Marshal(TokenBatch{Ids: ids})
		require.NoError(t, err)
		for _, path := range []string{"/api/token/batch", "/api/token/batch/keys"} {
			request := httptest.NewRequest("POST", path, bytes.NewReader(body))
			request.Header.Set("Authorization", "Bearer "+jwt)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			var event model.AuditLog
			require.NoError(t, model.LOG_DB.Where("request_id = ?", response.Header().Get(common.RequestIdKey)).First(&event).Error)
			var params struct {
				IDs       []int `json:"requested_ids"`
				Truncated bool  `json:"requested_ids_truncated"`
				Total     int   `json:"total"`
				Count     *int  `json:"count"`
			}
			encoded, err := common.Marshal(event.Other.Op.Params)
			require.NoError(t, err)
			require.NoError(t, common.Unmarshal(encoded, &params))
			assert.Equal(t, ids[:100], params.IDs)
			assert.True(t, params.Truncated)
			assert.Equal(t, 101, params.Total)
			assert.Equal(t, path == "/api/token/batch", event.Success)
			if event.Success {
				require.NotNil(t, params.Count)
				assert.Zero(t, *params.Count)
			} else {
				assert.Nil(t, params.Count)
			}
		}
	})

	t.Run("reads and unauthenticated writes add no operation audit", func(t *testing.T) {
		for _, tc := range []struct{ method, path, credential string }{
			{"GET", "/api/token/", jwt}, {"GET", "/api/token/search?keyword=private-search", jwt},
			{"GET", "/api/token/999999", jwt}, {"POST", "/api/token/", ""},
		} {
			response := auditRequest(router, tc.method, tc.path, tc.credential)
			var count int64
			require.NoError(t, model.LOG_DB.Model(&model.AuditLog{}).Where("request_id = ?", response.Header().Get(common.RequestIdKey)).Count(&count).Error)
			assert.Zero(t, count)
		}
	})

	t.Run("self audit excludes other owners", func(t *testing.T) {
		model.RecordAuditLog(nil, model.AuditLog{UserId: other.Id, Username: other.Username, ActorRole: common.RoleCommonUser, Category: model.AuditCategorySecurity, Action: "token.delete", Content: "other-user-audit", Success: true})
		response := auditRequest(router, "GET", "/api/audit/self?category=security&page_size=100", jwt)
		assert.Equal(t, 200, response.Code)
		assert.Contains(t, response.Body.String(), "token.create")
		assert.NotContains(t, response.Body.String(), "other-user-audit")
	})

	t.Run("audit storage failure preserves operation result", func(t *testing.T) {
		require.NoError(t, model.LOG_DB.Callback().Create().Before("gorm:create").Register("token-audit:log-fail", func(tx *gorm.DB) {
			if tx.Statement.Table == "audit_logs" {
				_ = tx.AddError(errors.New("audit unavailable"))
			}
		}))
		t.Cleanup(func() { require.NoError(t, model.LOG_DB.Callback().Create().Remove("token-audit:log-fail")) })
		request := httptest.NewRequest("POST", "/api/token/", strings.NewReader(`{"name":"audit-down","unlimited_quota":true}`))
		request.Header.Set("Authorization", "Bearer "+jwt)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.True(t, decodeAPIResponse(t, response).Success)
		var count int64
		require.NoError(t, model.DB.Model(&model.Token{}).Where("name = ?", "audit-down").Count(&count).Error)
		assert.EqualValues(t, 1, count)
	})
}

func TestDeveloperKeyUsageProjectsOnlyOwnedRecordedEvents(t *testing.T) {
	runDeveloperUsageProjectionFixture(t, setupTokenControllerTestDB(t))
}

func runDeveloperUsageProjectionFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	oldQuota, oldCurrency := common.QuotaPerUnit, *operation_setting.GetGeneralSetting()
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	t.Cleanup(func() { common.QuotaPerUnit = oldQuota; *operation_setting.GetGeneralSetting() = oldCurrency })
	logs := []model.Log{
		{UserId: 7, TokenId: 41, CreatedAt: 1100, Type: model.LogTypeConsume, Quota: 500000, ModelName: "model-a", RequestId: "task-request", Other: `{"task_id":"duplicated-public-id","settlement_verified":true,"usage_facts":{"seconds":10},"usage_units":{"seconds":"second"},"admin_info":{"task_record_id":7},"group_ratio":99,"root_info":{"upstream_key":"upstream-private"}}`},
		{UserId: 7, TokenId: 41, CreatedAt: 1200, Type: model.LogTypeConsume, Quota: 250000, ModelName: "model-a", RequestId: "task-request", Other: `{"task_id":"duplicated-public-id","actual_quota":750000,"usage_facts":{"seconds":12},"usage_units":{"seconds":"second"},"admin_info":{"task_record_id":7}}`},
		{UserId: 7, TokenId: 41, CreatedAt: 1300, Type: model.LogTypeRefund, Quota: 100000, ModelName: "model-a", RequestId: "task-request", Other: `{"task_id":"duplicated-public-id","actual_quota":650000,"usage_facts":{"seconds":8},"usage_units":{"seconds":"second"},"admin_info":{"task_record_id":7}}`},
		{UserId: 7, TokenId: 41, CreatedAt: 1350, Type: model.LogTypeConsume, ModelName: "model-a", RequestId: "second-task", Other: `{"task_id":"duplicated-public-id","settlement_verified":true,"usage_facts":{"seconds":3},"usage_units":{"seconds":"second"},"admin_info":{"task_record_id":8}}`},
		{UserId: 7, TokenId: 41, CreatedAt: 1400, Type: model.LogTypeConsume, Quota: 50000, ModelName: "model-a", RequestId: "failed-with-fee", Other: `{"violation_fee":true,"status_code":400}`},
		{UserId: 7, TokenId: 41, CreatedAt: 1500, Type: model.LogTypeConsume, ModelName: "chat", RequestId: "chat-request", PromptTokens: 40, CompletionTokens: 2, Other: `{"request_path":"/v1/chat/completions","admin_info":{"usage_billing_path":"billing-usage-openai"}}`},
		{UserId: 7, TokenId: 41, CreatedAt: 1550, Type: model.LogTypeError, RequestId: "failed-with-fee"},
		{UserId: 7, TokenId: 41, CreatedAt: 1600, Type: model.LogTypeConsume, Other: `{"task_id":"historical","usage_facts":{"seconds":999},"usage_units":{"seconds":"second"}}`},
		{UserId: 7, TokenId: 41, CreatedAt: 1700, Type: model.LogTypeSystem, Content: "BILLING_RESERVATION_HELD", Other: `{"billing_state":"HELD"}`},
		{UserId: 7, TokenId: 42, CreatedAt: 1500, Type: model.LogTypeConsume, Quota: 9000000, Other: `{"secret":"same-user-other-key"}`},
		{UserId: 8, TokenId: 41, CreatedAt: 1500, Type: model.LogTypeConsume, Quota: 9000000, Other: `{"secret":"other-user"}`},
		{UserId: 7, TokenId: 0, CreatedAt: 1500, Type: model.LogTypeConsume, Quota: 9000000},
		{UserId: 7, TokenId: 41, CreatedAt: 2000, Type: model.LogTypeConsume, Quota: 9000000},
	}
	require.NoError(t, db.Create(&logs).Error)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/key/usage?start=1000&end=2000", nil)
	ctx.Set("public_api_key", &model.Token{Id: 41, UserId: 7, Key: "caller-key-private"})
	ctx.Set("id", 8)
	ctx.Set("token_id", 42)
	GetDeveloperKeyUsage(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Object        string `json:"object"`
		Unit          string `json:"unit"`
		Start         int64  `json:"start_time"`
		End           int64  `json:"end_time"`
		Consumption   string `json:"recorded_consumption"`
		Refunds       string `json:"recorded_refunds"`
		Net           string `json:"recorded_net"`
		BillingEvents int64  `json:"billing_events"`
		ErrorEvents   int64  `json:"error_events"`
		Status        string `json:"component_status"`
		Components    []struct {
			Subject  string `json:"subject"`
			Unit     string `json:"unit"`
			Quantity string `json:"quantity"`
		} `json:"components"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "key.usage", response.Object)
	assert.Equal(t, "USD", response.Unit)
	assert.Equal(t, int64(1000), response.Start)
	assert.Equal(t, int64(2000), response.End)
	assert.Equal(t, "1.6", response.Consumption)
	assert.Equal(t, "0.2", response.Refunds)
	assert.Equal(t, "1.4", response.Net)
	assert.Equal(t, int64(7), response.BillingEvents)
	assert.Equal(t, int64(1), response.ErrorEvents)
	assert.Equal(t, "partial", response.Status)
	quantities := make(map[string]string)
	for _, component := range response.Components {
		quantities[component.Subject+":"+component.Unit] = component.Quantity
	}
	assert.Equal(t, map[string]string{"seconds:second": "11", "input_tokens:token": "40", "output_tokens:token": "2"}, quantities)
	for _, forbidden := range []string{"caller-key-private", "upstream-private", "same-user-other-key", "other-user", "group_ratio", "admin_info", "request_id", "token_id", "requests"} {
		assert.NotContains(t, recorder.Body.String(), forbidden)
	}
}

func developerUsageTestRequest(path string, token *model.Token) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, path, nil)
	if token != nil {
		ctx.Set("public_api_key", token)
	}
	GetDeveloperKeyUsage(ctx)
	return response
}

func TestDeveloperKeyUsageRejectsPrivilegedAndUnprovenComponents(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	oldCurrency := *operation_setting.GetGeneralSetting()
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	t.Cleanup(func() { *operation_setting.GetGeneralSetting() = oldCurrency })
	logs := []model.Log{
		{UserId: 7, TokenId: 41, CreatedAt: 1100, Type: model.LogTypeConsume, RequestId: "safe-task", Other: `{"task_id":"task","settlement_verified":true,"usage_facts":{"seconds":4,"api_key":1,"user_id":7,"provider_cost":2,"group_ratio":3,"bad.subject":2,"quality":"hd"},"usage_units":{"seconds":"second","api_key":"count","user_id":"count","provider_cost":"credit","group_ratio":"credit","bad.subject":"count"},"admin_info":{"task_record_id":9}}`},
		{UserId: 7, TokenId: 41, CreatedAt: 1200, Type: model.LogTypeConsume, RequestId: "image", PromptTokens: 1, CompletionTokens: 1000, Other: `{"request_path":"/v1/images/generations","admin_info":{"usage_billing_path":"billing-usage-openai"}}`},
		{UserId: 7, TokenId: 41, CreatedAt: 1300, Type: model.LogTypeConsume, RequestId: "chat-estimate", PromptTokens: 999, CompletionTokens: 999, Other: `{"request_path":"/v1/chat/completions","admin_info":{"usage_billing_path":"upstream"}}`},
		{UserId: 7, TokenId: 41, CreatedAt: 1400, Type: model.LogTypeConsume, RequestId: "missing-units", Other: `{"task_id":"old","settlement_verified":true,"usage_facts":{"seconds":10},"admin_info":{"task_record_id":10}}`},
		{UserId: 7, TokenId: 41, CreatedAt: 1500, Type: model.LogTypeConsume, RequestId: "pending", Other: `{"task_id":"pending","billing_state":"PENDING","settlement_verified":true,"usage_facts":{"seconds":100},"usage_units":{"seconds":"second"},"admin_info":{"task_record_id":11}}`},
	}
	require.NoError(t, db.Create(&logs).Error)
	response := developerUsageTestRequest("/v1/key/usage?start=1000&end=2000", &model.Token{Id: 41, UserId: 7})
	require.Equal(t, http.StatusOK, response.Code)
	var usage dto.DeveloperKeyUsage
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &usage))
	assert.Equal(t, []dto.DeveloperUsageComponent{{Subject: "seconds", Unit: "second", Quantity: "4"}}, usage.Components)
	assert.Equal(t, "partial", usage.ComponentStatus)
	for _, forbidden := range []string{"api_key", "user_id", "provider_cost", "group_ratio", "bad.subject", "input_tokens", "output_tokens"} {
		assert.NotContains(t, response.Body.String(), forbidden)
	}
}

func TestDeveloperKeyUsageBoundsEmptyWindowAndDisplayUnits(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	oldCurrency := *operation_setting.GetGeneralSetting()
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	t.Cleanup(func() { *operation_setting.GetGeneralSetting() = oldCurrency })
	token := &model.Token{Id: 41, UserId: 7, RemainQuota: 0}
	for _, query := range []string{"start=oops", "end=oops", "start=-1", "end=-1", "start=1&end=1", "start=2&end=1", "start=0&end=2678401", "start=1&start=2&end=3", "start=1&end=2&end=3", "start=1&end=2&model=a&model=b"} {
		t.Run(query, func(t *testing.T) {
			response := developerUsageTestRequest("/v1/key/usage?"+query, token)
			assert.Equal(t, http.StatusBadRequest, response.Code)
		})
	}
	assert.Equal(t, http.StatusUnauthorized, developerUsageTestRequest("/v1/key/usage", nil).Code)
	before := common.GetTimestamp()
	response := developerUsageTestRequest("/v1/key/usage", token)
	require.Equal(t, http.StatusOK, response.Code)
	var usage dto.DeveloperKeyUsage
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &usage))
	assert.Equal(t, int64(30*24*60*60), usage.EndTime-usage.StartTime)
	assert.GreaterOrEqual(t, usage.EndTime, before+1)
	assert.LessOrEqual(t, usage.EndTime, common.GetTimestamp()+1)
	assert.Equal(t, "0", usage.RecordedConsumption)
	assert.Equal(t, "0", usage.RecordedRefunds)
	assert.Equal(t, "0", usage.RecordedNet)
	assert.Empty(t, usage.Components)
	assert.Contains(t, response.Body.String(), `"components":[]`)
	assert.Equal(t, "available", usage.ComponentStatus)
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	assert.Equal(t, http.StatusInternalServerError, developerUsageTestRequest("/v1/key/usage", token).Code)
}

func TestDeveloperKeyUsageDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var mainDriver, logDriver gorm.Dialector
			switch dialect {
			case "sqlite":
				mainDriver = sqlite.Open(":memory:")
				logDriver = sqlite.Open(":memory:")
			case "mysql":
				mainDSN, logDSN := os.Getenv("PHASE3_MYSQL_DSN"), os.Getenv("PHASE3_MYSQL_LOG_DSN")
				if mainDSN == "" || logDSN == "" {
					t.Skip("isolated PHASE3_MYSQL_DSN and PHASE3_MYSQL_LOG_DSN required")
				}
				mainDriver = mysql.Open(mainDSN)
				logDriver = mysql.Open(logDSN)
			case "postgres":
				mainDSN, logDSN := os.Getenv("PHASE3_POSTGRES_DSN"), os.Getenv("PHASE3_POSTGRES_LOG_DSN")
				if mainDSN == "" || logDSN == "" {
					t.Skip("isolated PHASE3_POSTGRES_DSN and PHASE3_POSTGRES_LOG_DSN required")
				}
				mainDriver = postgres.New(postgres.Config{DSN: mainDSN, PreferSimpleProtocol: true})
				logDriver = postgres.New(postgres.Config{DSN: logDSN, PreferSimpleProtocol: true})
			}
			mainDB, err := gorm.Open(mainDriver, &gorm.Config{})
			require.NoError(t, err)
			logDB, err := gorm.Open(logDriver, &gorm.Config{})
			require.NoError(t, err)
			oldDB, oldLogDB := model.DB, model.LOG_DB
			oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
			model.DB, model.LOG_DB = mainDB, logDB
			kind := common.DatabaseTypeSQLite
			if dialect == "mysql" {
				kind = common.DatabaseTypeMySQL
			}
			if dialect == "postgres" {
				kind = common.DatabaseTypePostgreSQL
			}
			common.SetDatabaseTypes(kind, kind)
			t.Cleanup(func() {
				model.DB, model.LOG_DB = oldDB, oldLogDB
				common.SetDatabaseTypes(oldMain, oldLog)
				for _, db := range []*gorm.DB{mainDB, logDB} {
					sqlDB, err := db.DB()
					require.NoError(t, err)
					require.NoError(t, sqlDB.Close())
				}
			})
			require.False(t, logDB.Migrator().HasTable(&model.Log{}), "matrix requires an isolated empty log store")
			t.Cleanup(func() { require.NoError(t, logDB.Migrator().DropTable(&model.Log{})) })
			require.NoError(t, logDB.AutoMigrate(&model.Log{}))
			runDeveloperUsageProjectionFixture(t, logDB)
			versionQuery := "SELECT version()"
			if dialect == "sqlite" {
				versionQuery = "SELECT sqlite_version()"
			}
			var version string
			require.NoError(t, logDB.Raw(versionQuery).Scan(&version).Error)
			t.Logf("separate log database version: %s", version)
			var plan []map[string]any
			explain := "EXPLAIN "
			if dialect == "sqlite" {
				explain = "EXPLAIN QUERY PLAN "
			}
			require.NoError(t, logDB.Raw(explain+"SELECT SUM(quota) FROM logs WHERE user_id = ? AND token_id = ? AND created_at >= ? AND created_at < ?", 7, 41, 1000, 2000).Find(&plan).Error)
			t.Logf("owned usage aggregate query plan: %v", plan)
		})
	}
}

func TestDeveloperKeyUsageBoundsComponentRowsWithoutTruncatingMoney(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	oldQuota, oldCurrency := common.QuotaPerUnit, *operation_setting.GetGeneralSetting()
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	t.Cleanup(func() { common.QuotaPerUnit = oldQuota; *operation_setting.GetGeneralSetting() = oldCurrency })
	logs := make([]model.Log, 10001)
	for i := range logs {
		logs[i] = model.Log{UserId: 7, TokenId: 41, CreatedAt: 1100, Type: model.LogTypeConsume, Quota: 1, RequestId: fmt.Sprint("request_", i), Other: fmt.Sprintf(`{"task_id":"task_%d","settlement_verified":true,"usage_facts":{"image_count":1},"usage_units":{"image_count":"count"},"admin_info":{"task_record_id":%d}}`, i, i+1)}
	}
	require.NoError(t, db.CreateInBatches(&logs, 100).Error)
	queries := 0
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register("usage:bounded-query-count", func(tx *gorm.DB) { queries++ }))
	t.Cleanup(func() { require.NoError(t, db.Callback().Row().Remove("usage:bounded-query-count")) })
	response := developerUsageTestRequest("/v1/key/usage?start=1000&end=2000", &model.Token{Id: 41, UserId: 7})
	require.Equal(t, http.StatusOK, response.Code)
	var usage dto.DeveloperKeyUsage
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &usage))
	assert.Equal(t, int64(10001), usage.BillingEvents)
	assert.Equal(t, "0.020002", usage.RecordedConsumption)
	assert.Equal(t, "partial", usage.ComponentStatus)
	assert.Equal(t, []dto.DeveloperUsageComponent{{Subject: "image_count", Unit: "count", Quantity: "10000"}}, usage.Components)
	assert.Equal(t, 2, queries, "one database aggregate and one bounded component stream")
}

func TestDeveloperKeyUsageUnitAndSettlementEvidence(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	oldCurrency := *operation_setting.GetGeneralSetting()
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	t.Cleanup(func() { *operation_setting.GetGeneralSetting() = oldCurrency })
	for _, tc := range []struct {
		name   string
		other  []string
		kinds  []int
		want   []dto.DeveloperUsageComponent
		status string
	}{
		{name: "premature image synchronous presentation", other: []string{`{"task_id":"image","task_sync":true,"usage_facts":{"image_count":5},"usage_units":{"image_count":"count"},"admin_info":{"task_record_id":9}}`}, kinds: []int{model.LogTypeConsume}, want: []dto.DeveloperUsageComponent{}, status: "unknown"},
		{name: "verified actual image count", other: []string{`{"task_id":"image","settlement_verified":true,"usage_facts":{"image_count":2},"usage_units":{"image_count":"count"},"admin_info":{"task_record_id":9}}`}, kinds: []int{model.LogTypeConsume}, want: []dto.DeveloperUsageComponent{{Subject: "image_count", Unit: "count", Quantity: "2"}}, status: "available"},
		{name: "full failure refund does not preserve estimated quantity", other: []string{`{"task_id":"image","settlement_verified":true,"usage_facts":{"image_count":2},"usage_units":{"image_count":"count"},"admin_info":{"task_record_id":9}}`, `{"task_id":"image","usage_facts":{"image_count":2},"usage_units":{"image_count":"count"},"admin_info":{"task_record_id":9}}`}, kinds: []int{model.LogTypeConsume, model.LogTypeRefund}, want: []dto.DeveloperUsageComponent{}, status: "unknown"},
		{name: "frozen plugin dimensions preserve distinct units", other: []string{`{"task_id":"media","settlement_verified":true,"usage_facts":{"seconds":10,"image_count":2,"tokens":123,"units":1.5,"characters":6,"generate_audio":true,"mode":"hd"},"usage_units":{"seconds":"second","image_count":"count","tokens":"token","units":"credit","characters":"character"},"admin_info":{"task_record_id":9}}`}, kinds: []int{model.LogTypeConsume}, want: []dto.DeveloperUsageComponent{{Subject: "characters", Unit: "character", Quantity: "6"}, {Subject: "image_count", Unit: "count", Quantity: "2"}, {Subject: "seconds", Unit: "second", Quantity: "10"}, {Subject: "tokens", Unit: "token", Quantity: "123"}, {Subject: "units", Unit: "credit", Quantity: "1.5"}}, status: "available"},
		{name: "explicit measured cache and modality dimensions", other: []string{`{"request_path":"/v1/chat/completions","billing_tokens":{"p":40,"c":2,"cr":10,"cc":0,"img":5,"ai":3,"ao":1},"admin_info":{"usage_billing_path":"billing-usage-openai"}}`}, kinds: []int{model.LogTypeConsume}, want: []dto.DeveloperUsageComponent{{Subject: "audio_input_tokens", Unit: "token", Quantity: "3"}, {Subject: "audio_output_tokens", Unit: "token", Quantity: "1"}, {Subject: "billable_input_tokens", Unit: "token", Quantity: "40"}, {Subject: "billable_output_tokens", Unit: "token", Quantity: "2"}, {Subject: "cache_creation_tokens", Unit: "token", Quantity: "0"}, {Subject: "cache_read_tokens", Unit: "token", Quantity: "10"}, {Subject: "image_input_tokens", Unit: "token", Quantity: "5"}}, status: "available"},
		{name: "recorded cache total excludes duplicated TTL breakdown", other: []string{`{"request_path":"/v1/messages","cache_tokens":9,"cache_creation_tokens":17,"cache_creation_tokens_5m":10,"cache_creation_tokens_1h":7,"admin_info":{"usage_billing_path":"billing-usage-anthropic"}}`}, kinds: []int{model.LogTypeConsume}, want: []dto.DeveloperUsageComponent{{Subject: "cache_creation_tokens", Unit: "token", Quantity: "17"}, {Subject: "cache_read_tokens", Unit: "token", Quantity: "9"}, {Subject: "input_tokens", Unit: "token", Quantity: "40"}, {Subject: "output_tokens", Unit: "token", Quantity: "2"}}, status: "available"},
		{name: "recorded TTL cache facts without invented total", other: []string{`{"request_path":"/v1/messages","cache_tokens":3,"cache_creation_tokens_5m":10,"cache_creation_tokens_1h":7,"admin_info":{"usage_billing_path":"billing-usage-anthropic"}}`}, kinds: []int{model.LogTypeConsume}, want: []dto.DeveloperUsageComponent{{Subject: "cache_creation_1h_tokens", Unit: "token", Quantity: "7"}, {Subject: "cache_creation_5m_tokens", Unit: "token", Quantity: "10"}, {Subject: "cache_read_tokens", Unit: "token", Quantity: "3"}, {Subject: "input_tokens", Unit: "token", Quantity: "40"}, {Subject: "output_tokens", Unit: "token", Quantity: "2"}}, status: "available"},
		{name: "absent cache quantity stays absent", other: []string{`{"request_path":"/v1/chat/completions","admin_info":{"usage_billing_path":"billing-usage-openai"}}`}, kinds: []int{model.LogTypeConsume}, want: []dto.DeveloperUsageComponent{{Subject: "input_tokens", Unit: "token", Quantity: "40"}, {Subject: "output_tokens", Unit: "token", Quantity: "2"}}, status: "available"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, db.Where("user_id = ?", 7).Delete(&model.Log{}).Error)
			for i, other := range tc.other {
				row := model.Log{UserId: 7, TokenId: 41, CreatedAt: int64(1100 + i), Type: tc.kinds[i], RequestId: "request", PromptTokens: 40, CompletionTokens: 2, Other: other}
				require.NoError(t, db.Create(&row).Error)
			}
			response := developerUsageTestRequest("/v1/key/usage?start=1000&end=2000", &model.Token{Id: 41, UserId: 7})
			require.Equal(t, http.StatusOK, response.Code)
			var usage dto.DeveloperKeyUsage
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &usage))
			assert.Equal(t, tc.want, usage.Components)
			assert.Equal(t, tc.status, usage.ComponentStatus)
		})
	}
}
