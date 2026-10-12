package model

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestTaskCreationPersistsAuthenticatedTokenOwnership(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, database.AutoMigrate(&Task{}))
	task := InitTask(constant.TaskPlatform("test-plugin"), &relaycommon.RelayInfo{UserId: 7, TokenId: 41, CanonicalRequestId: "http_original", RequestId: "billing_changed", ChannelMeta: &relaycommon.ChannelMeta{}})
	require.NoError(t, database.Create(task).Error)
	var persisted map[string]any
	require.NoError(t, database.Model(&Task{}).Where("id = ?", task.ID).Take(&persisted).Error)
	assert.EqualValues(t, 41, persisted["token_id"], "task ownership must be durable independently of billing state")
	assert.Equal(t, "http_original", persisted["request_id"])
	assert.Equal(t, 41, task.PrivateData.TokenId)
	assert.True(t, task.HasCanonicalOwnership())
	body, err := common.Marshal(task)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "token_id")
	assert.NotContains(t, string(body), "request_id")
}

func TestTaskOwnershipDatabaseMigration(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		for _, upgrade := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/upgrade=%t", dialect, upgrade), func(t *testing.T) {
				database := openTaskOwnershipDatabase(t, dialect)
				taskStatement := &gorm.Statement{DB: database}
				require.NoError(t, taskStatement.Parse(&Task{}))
				mjStatement := &gorm.Statement{DB: database}
				require.NoError(t, mjStatement.Parse(&Midjourney{}))
				require.NoError(t, database.AutoMigrate(&Token{}))
				if upgrade {
					// v1.0.0-rc.41 and 319bd4d47 have identical Task/Midjourney
					// column definitions. Remove only this migration's additions
					// to instantiate their full pre-change schema and old indexes.
					require.NoError(t, database.Table(taskStatement.Table).AutoMigrate(taskOwnershipLegacyModel(Task{}, "TokenId", "RequestId")))
					require.NoError(t, database.Table(mjStatement.Table).AutoMigrate(taskOwnershipLegacyModel(Midjourney{}, "OwnerTokenId", "RequestId")))
					assert.False(t, database.Migrator().HasColumn(&Task{}, "token_id"))
					assert.False(t, database.Migrator().HasColumn(&Midjourney{}, "owner_token_id"))
				} else {
					require.NoError(t, database.AutoMigrate(&Task{}, &Midjourney{}))
				}
				tokens := []Token{{Id: 41, UserId: 7, Key: "ownership_a"}, {Id: 42, UserId: 7, Key: "ownership_b"}, {Id: 43, UserId: 8, Key: "ownership_c"}}
				require.NoError(t, database.Create(&tokens).Error)
				require.NoError(t, database.Delete(&tokens[1]).Error)
				fixtures := []struct {
					name        string
					user        int
					private     string
					wantToken   int
					wantRequest string
				}{
					{"ordinary", 7, `{"token_id":41,"execution":{"request_id":"http_original"}}`, 41, "http_original"},
					{"duplicate", 7, `{"token_id":41}`, 41, ""},
					{"duplicate", 7, `{"token_id":42}`, 42, ""},
					{"deleted_token", 7, `{"token_id":42,"execution":{"request_id":"http_deleted"}}`, 42, "http_deleted"},
					{"wrong_user", 7, `{"token_id":43,"execution":{"request_id":"http_wrong"}}`, 0, ""},
					{"missing_token", 7, `{"token_id":999,"execution":{"request_id":"http_missing"}}`, 0, ""},
					{"unknown", 7, `{"execution":{"request_id":"http_unknown"}}`, 0, ""},
					{"billing_digest", 7, `{"token_id":41,"execution":{"request_id":"` + strings.Repeat("a", 64) + `"}}`, 41, ""},
					{"canonical_digest", 7, `{"token_id":41,"execution":{"request_id":"` + strings.Repeat("a", 64) + `","canonical_request_id":"http_canonical"}}`, 41, "http_canonical"},
					{"internal_verification", 7, `{"token_id":41,"execution":{"runtime_verification_item_id":9,"request_id":"http_internal"}}`, 0, ""},
					{"oversized_request", 7, `{"token_id":41,"execution":{"request_id":"` + strings.Repeat("x", 65) + `"}}`, 41, ""},
				}
				for i, fixture := range fixtures {
					require.NoError(t, database.Table(taskStatement.Table).Create(map[string]any{"id": i + 1, "task_id": fixture.name, "user_id": fixture.user, "quota": 123, "private_data": fixture.private, "data": `{"safe":"unchanged"}`}).Error)
				}
				// Cross a complete migration batch; the final row must not be missed.
				for i := range 201 {
					require.NoError(t, database.Table(taskStatement.Table).Create(map[string]any{"id": i + 100, "task_id": fmt.Sprintf("batch_%d", i), "user_id": 7, "private_data": `{"token_id":41}`}).Error)
				}
				for i, tokenID := range []int{41, 42, 43, 0} {
					require.NoError(t, database.Table(mjStatement.Table).Create(map[string]any{"id": i + 1, "user_id": 7, "mj_id": "duplicate_mj", "token_id": tokenID, "quota": 123}).Error)
				}
				if dialect == "sqlite" {
					require.NoError(t, database.Table(taskStatement.Table).Create(map[string]any{"id": 500, "task_id": "malformed", "user_id": 7, "private_data": `{"token_id":41,"execution":`}).Error)
				}
				require.NoError(t, database.AutoMigrate(&Task{}, &Midjourney{}))
				// An existing assignment is never overwritten by backfill evidence.
				require.NoError(t, database.Model(&Task{}).Where("id = ?", 2).Updates(map[string]any{"token_id": 42, "request_id": "already_canonical"}).Error)
				for range 2 {
					require.NoError(t, database.AutoMigrate(&Task{}, &Midjourney{}))
					require.NoError(t, MigrateTaskOwnership(database))
				}
				for i, fixture := range fixtures {
					var task Task
					require.NoError(t, database.Omit("data").First(&task, i+1).Error)
					if i == 1 {
						assert.Equal(t, 42, task.TokenId)
						assert.Equal(t, "already_canonical", task.RequestId)
						assert.False(t, task.HasCanonicalOwnership(), "contradictory ownership must fail closed")
					} else {
						assert.Equal(t, fixture.wantToken, task.TokenId, fixture.name)
						assert.Equal(t, fixture.wantRequest, task.RequestId, fixture.name)
						assert.Equal(t, fixture.wantToken > 0, task.HasCanonicalOwnership(), fixture.name)
					}
					assert.Equal(t, 123, task.Quota)
					var originalPrivate TaskPrivateData
					require.NoError(t, common.UnmarshalJsonStr(fixture.private, &originalPrivate))
					assert.Equal(t, originalPrivate, task.PrivateData, "backfill must preserve billing and execution history")
					var originalData struct{ Data string }
					require.NoError(t, database.Model(&Task{}).Select("data").Where("id = ?", task.ID).Take(&originalData).Error)
					var payload map[string]string
					require.NoError(t, common.UnmarshalJsonStr(originalData.Data, &payload))
					assert.Equal(t, "unchanged", payload["safe"])
				}
				var last Task
				require.NoError(t, database.First(&last, 300).Error)
				assert.Equal(t, 41, last.TokenId)
				var midjourneys []Midjourney
				require.NoError(t, database.Order("id").Find(&midjourneys).Error)
				require.Len(t, midjourneys, 4)
				assert.Equal(t, []int{41, 42, 0, 0}, []int{midjourneys[0].OwnerTokenId, midjourneys[1].OwnerTokenId, midjourneys[2].OwnerTokenId, midjourneys[3].OwnerTokenId})
				for i, tokenID := range []int{41, 42, 43, 0} {
					assert.Equal(t, tokenID, midjourneys[i].TokenId)
					assert.Equal(t, 123, midjourneys[i].Quota)
					assert.Empty(t, midjourneys[i].RequestId)
				}
				assert.True(t, database.Migrator().HasIndex(&Task{}, "TaskID"), "old lookup index must survive")
				assert.True(t, database.Migrator().HasIndex(&Task{}, "idx_tasks_token_id_id"))
				assert.True(t, database.Migrator().HasIndex(&Task{}, "idx_tasks_token_id_task_id"))
				if dialect == "mysql" {
					var rowFormat string
					require.NoError(t, database.Raw("SELECT ROW_FORMAT FROM information_schema.tables WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?", taskStatement.Table).Scan(&rowFormat).Error)
					assert.Equal(t, "Compact", rowFormat, "upgrade must preserve the legacy row format")
				}
				if dialect == "sqlite" {
					var row struct {
						TokenId   int
						RequestId string
					}
					require.NoError(t, database.Model(&Task{}).Select("token_id", "request_id").Where("id = ?", 500).Take(&row).Error)
					assert.Zero(t, row.TokenId)
					assert.Empty(t, row.RequestId)
				}
			})
		}
	}
}

func openTaskOwnershipDatabase(t *testing.T, dialect string) *gorm.DB {
	t.Helper()
	var driver gorm.Dialector
	switch dialect {
	case "sqlite":
		driver = sqlite.Open(":memory:")
	case "mysql":
		dsn := os.Getenv("PHASE3_MYSQL_DSN")
		if dsn == "" {
			t.Skip("PHASE3_MYSQL_DSN not configured")
		}
		driver = mysql.Open(dsn)
	case "postgres":
		dsn := os.Getenv("PHASE3_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("PHASE3_POSTGRES_DSN not configured")
		}
		driver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
	}
	database, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: fmt.Sprintf("ownership_%d_", time.Now().UnixNano())}})
	require.NoError(t, err)
	if dialect == "mysql" {
		// Released installations may retain COMPACT tables and their 767-byte
		// index limit even when the server has newer defaults.
		database = database.Set("gorm:table_options", "ENGINE=InnoDB ROW_FORMAT=COMPACT CHARSET=utf8mb4").Session(&gorm.Session{})
	}
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		require.NoError(t, database.Migrator().DropTable(&Task{}, &Midjourney{}, &Token{}))
		require.NoError(t, sqlDB.Close())
	})
	var version string
	if dialect == "sqlite" {
		require.NoError(t, database.Raw("SELECT sqlite_version()").Scan(&version).Error)
	} else {
		require.NoError(t, database.Raw("SELECT version()").Scan(&version).Error)
	}
	t.Logf("database: %s %s", dialect, version)
	return database
}

// taskOwnershipLegacyModel reproduces the full pre-change column definitions
// without depending on root-only generated fixtures or changing public models.
func taskOwnershipLegacyModel(value any, omit ...string) any {
	modelType := reflect.TypeOf(value)
	fields := make([]reflect.StructField, 0, modelType.NumField())
	for i := range modelType.NumField() {
		field := modelType.Field(i)
		omitted := false
		for _, name := range omit {
			if field.Name == name {
				omitted = true
				break
			}
		}
		if omitted {
			continue
		}
		field.Tag = reflect.StructTag(strings.ReplaceAll(strings.ReplaceAll(string(field.Tag), ";index:idx_tasks_token_id_id,priority:2", ""), ";index:idx_tasks_token_id_task_id,priority:2", ""))
		fields = append(fields, field)
	}
	return reflect.New(reflect.StructOf(fields)).Interface()
}
