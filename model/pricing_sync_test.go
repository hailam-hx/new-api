package model

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	_ "github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func verifyPricingSyncLifecycle(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&Option{Key: "dflop-unrelated-existing", Value: "preserved"}).Error)
	require.NoError(t, DB.AutoMigrate(&Option{}, &Channel{}, &Ability{}, &Model{}, &Vendor{}, &PricingSyncRun{}, &PricingSyncItem{}, &PricingSyncManaged{}))
	require.NoError(t, DB.AutoMigrate(&Option{}, &Channel{}, &Ability{}, &Model{}, &Vendor{}, &PricingSyncRun{}, &PricingSyncItem{}, &PricingSyncManaged{}))
	var existing Option
	require.NoError(t, DB.Where(commonKeyCol+" = ?", "dflop-unrelated-existing").First(&existing).Error)
	assert.Equal(t, "preserved", existing.Value)
	InitOptionMap()
	config := DefaultDFLOPConfig()
	config.Enabled = true
	config.CNYToUSD = "0.15"
	require.NoError(t, SaveDFLOPConfig(config))
	unique := time.Now().UnixNano()
	name := fmt.Sprintf("dflop-sync-test-model-%d", unique)
	previousKeyCol := commonKeyCol
	commonKeyCol = "" // Standalone read-only tools do not call InitDB.
	previous, err := GetModelPricingSnapshot([]string{name})
	commonKeyCol = previousKeyCol
	require.NoError(t, err)
	version := previous.Entries[0].Version
	batchVersion := ModelPricingVersion(PricingValues{"versions": map[string]string{name: version}})
	proposed := PricingValues{"billing_setting.billing_mode": "tiered_expr", "billing_setting.billing_expr": `tier("dflop", p * 1 + c * 2)`}
	encoded, err := common.Marshal(proposed)
	require.NoError(t, err)
	run := PricingSyncRun{ID: fmt.Sprintf("pricing-sync-test-%d", unique), Provider: "dflop", Status: "preview", ConfigHash: config.Hash(), SourceHash: "source", PricingVersionBefore: batchVersion}
	item := PricingSyncItem{RunID: run.ID, ModelID: name, Status: "SUPPORTED_AUTO", Action: "ADD", ExpectedVersion: version, ProposedPricing: string(encoded)}
	require.NoError(t, CreatePricingSyncPreview(&run, []PricingSyncItem{item}))
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ApplyPricingSyncWithContext(cancelled, run.ID, config.Hash(), "source", []string{name}, false, 5)
	require.Error(t, err)
	_, err = ApplyPricingSync(run.ID, "wrong", "source", []string{name}, false, 5)
	require.ErrorIs(t, err, ErrModelPricingConflict)
	require.NoError(t, RecordPricingSyncApplyFailure(run.ID, err))
	failedPreview, _, err := GetPricingSyncPreview(run.ID)
	require.NoError(t, err)
	assert.Contains(t, failedPreview.ErrorMessage, ErrModelPricingConflict.Error())
	applied, err := ApplyPricingSync(run.ID, config.Hash(), "source", []string{name}, false, 5)
	require.NoError(t, err)
	assert.Equal(t, 5, applied.AppliedBy)
	after, err := GetModelPricingSnapshot([]string{name})
	require.NoError(t, err)
	assert.Equal(t, proposed["billing_setting.billing_expr"], after.Entries[0].Configured["billing_setting.billing_expr"])
	_, err = ApplyPricingSync(run.ID, config.Hash(), "source", []string{name}, false, 5)
	require.Error(t, err)
	_, err = RollbackPricingSync(run.ID, 1)
	require.NoError(t, err)
	restored, err := GetModelPricingSnapshot([]string{name})
	require.NoError(t, err)
	assert.Equal(t, version, restored.Entries[0].Version)
	assert.False(t, errors.Is(err, ErrModelPricingConflict))
	second := PricingSyncRun{ID: run.ID + "-second", Provider: "dflop", Status: "preview", ConfigHash: config.Hash(), SourceHash: "source", PricingVersionBefore: batchVersion}
	item.RunID = second.ID
	require.NoError(t, CreatePricingSyncPreview(&second, []PricingSyncItem{item}))
	firstPage, err := ListPricingSyncRunsPage(1, 0)
	require.NoError(t, err)
	secondPage, err := ListPricingSyncRunsPage(1, 1)
	require.NoError(t, err)
	require.Len(t, firstPage, 1)
	require.Len(t, secondPage, 1)
	assert.NotEqual(t, firstPage[0].ID, secondPage[0].ID)
	_, err = ApplyPricingSync(second.ID, config.Hash(), "source", []string{name}, false, 5)
	require.NoError(t, err)
	current, err := GetModelPricingSnapshot([]string{name})
	require.NoError(t, err)
	manual := PricingValues{"billing_setting.billing_mode": "tiered_expr", "billing_setting.billing_expr": `tier("custom", p * 5)`}
	require.NoError(t, UpdateModelPricing([]ModelPricingChange{{ModelName: name, ExpectedVersion: current.Entries[0].Version, Pricing: manual}}))
	_, err = RollbackPricingSync(second.ID, 1)
	require.ErrorIs(t, err, ErrModelPricingConflict)
	current, err = GetModelPricingSnapshot([]string{name})
	require.NoError(t, err)
	assert.Equal(t, manual["billing_setting.billing_expr"], current.Entries[0].Configured["billing_setting.billing_expr"])

	pluginModel := "doubao-seedream-4-0-250828"
	pluginBefore, err := GetModelPricingSnapshot([]string{pluginModel})
	require.NoError(t, err)
	pluginVersion := pluginBefore.Entries[0].Version
	pluginExpression := `tier("image", u("image_count") * 0.2)`
	pluginProposed := PricingValues{billing_setting.PluginBillingExprOption: map[string]any{"doubao": pluginExpression}}
	pluginEncoded, err := common.Marshal(pluginProposed)
	require.NoError(t, err)
	pluginRun := PricingSyncRun{ID: fmt.Sprintf("pricing-sync-plugin-%d", unique), Provider: "dflop", Status: "preview", ConfigHash: config.Hash(), SourceHash: "source", PricingVersionBefore: ModelPricingVersion(PricingValues{"versions": map[string]string{pluginModel: pluginVersion}})}
	pluginItem := PricingSyncItem{RunID: pluginRun.ID, ModelID: pluginModel, Status: "SUPPORTED_AUTO", Action: "ADD", ExpectedVersion: pluginVersion, ProposedPricing: string(pluginEncoded), PricingScope: "PLUGIN_OVERRIDE", PluginKey: "doubao", PricingShape: "image:per_image"}
	require.NoError(t, CreatePricingSyncPreview(&pluginRun, []PricingSyncItem{pluginItem}))
	_, err = ApplyPricingSync(pluginRun.ID, config.Hash(), "source", []string{pluginModel}, false, 5)
	require.NoError(t, err)
	resolved, ok := billing_setting.ResolveTaskBillingExpr("doubao", pluginModel, pluginModel)
	assert.True(t, ok)
	assert.Equal(t, pluginExpression, resolved)
	pluginAfter, err := GetModelPricingSnapshot([]string{pluginModel})
	require.NoError(t, err)
	assert.Equal(t, pluginExpression, pluginAfter.Entries[0].Configured[billing_setting.PluginBillingExprOption].(map[string]any)["doubao"])
	_, err = RollbackPricingSync(pluginRun.ID, 5)
	require.NoError(t, err)
	pluginRestored, err := GetModelPricingSnapshot([]string{pluginModel})
	require.NoError(t, err)
	assert.Equal(t, pluginVersion, pluginRestored.Entries[0].Version)
}

func TestPricingSyncDatabaseMatrix(t *testing.T) {
	originalDB := DB
	originalMainType, originalLogType := common.MainDatabaseType(), common.LogDatabaseType()
	t.Cleanup(func() { DB = originalDB; common.SetDatabaseTypes(originalMainType, originalLogType); initCol() })
	for _, tc := range []struct {
		name, env    string
		dialect      gorm.Dialector
		databaseType common.DatabaseType
	}{
		{"sqlite", "", sqlite.Open(":memory:"), common.DatabaseTypeSQLite},
		{"mysql", "DFLOP_TEST_MYSQL_DSN", mysql.Open(os.Getenv("DFLOP_TEST_MYSQL_DSN")), common.DatabaseTypeMySQL},
		{"postgres", "DFLOP_TEST_PG_DSN", postgres.Open(os.Getenv("DFLOP_TEST_PG_DSN")), common.DatabaseTypePostgreSQL},
	} {
		if tc.env != "" && os.Getenv(tc.env) == "" {
			t.Logf("%s DSN not set; external database case omitted", tc.name)
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			db, err := gorm.Open(tc.dialect, &gorm.Config{})
			require.NoError(t, err)
			DB = db
			var databaseVersion string
			versionQuery := "SELECT version()"
			if tc.name == "sqlite" {
				versionQuery = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(versionQuery).Scan(&databaseVersion).Error)
			t.Logf("database version: %s", databaseVersion)
			common.SetDatabaseTypes(tc.databaseType, tc.databaseType)
			initCol()
			require.NoError(t, migrateDB())
			require.NoError(t, migrateDB())
			verifyPricingSyncLifecycle(t)
		})
	}
}

func TestDFLOPConfigDefaultsRequireExplicitRate(t *testing.T) {
	config := DefaultDFLOPConfig()
	assert.False(t, config.Enabled)
	assert.False(t, config.AutoApplyEnabled)
	require.Error(t, config.ValidateForPreview())
	config.CNYToUSD = "0.15"
	require.NoError(t, config.ValidateForPreview())
	config.SyncIntervalHours = 0
	require.Error(t, config.Validate())
}

// These DSNs point to disposable databases seeded by the latest released
// version. Run separately from the fresh-database matrix.
func TestPricingSyncReleasedUpgrade(t *testing.T) {
	originalDB := DB
	originalMainType, originalLogType := common.MainDatabaseType(), common.LogDatabaseType()
	t.Cleanup(func() { DB = originalDB; common.SetDatabaseTypes(originalMainType, originalLogType); initCol() })
	for _, tc := range []struct {
		name, env    string
		open         func(string) gorm.Dialector
		databaseType common.DatabaseType
	}{
		{"sqlite", "DFLOP_TEST_UPGRADE_SQLITE", func(dsn string) gorm.Dialector { return sqlite.Open(dsn) }, common.DatabaseTypeSQLite},
		{"mysql", "DFLOP_TEST_UPGRADE_MYSQL_DSN", func(dsn string) gorm.Dialector { return mysql.Open(dsn) }, common.DatabaseTypeMySQL},
		{"postgres", "DFLOP_TEST_UPGRADE_PG_DSN", func(dsn string) gorm.Dialector { return postgres.Open(dsn) }, common.DatabaseTypePostgreSQL},
	} {
		dsn := os.Getenv(tc.env)
		if dsn == "" {
			t.Logf("%s released database not configured", tc.name)
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			db, err := gorm.Open(tc.open(dsn), &gorm.Config{})
			require.NoError(t, err)
			DB = db
			common.SetDatabaseTypes(tc.databaseType, tc.databaseType)
			initCol()
			require.NoError(t, migrateDB())
			require.NoError(t, migrateDB())
			var preserved Option
			require.NoError(t, DB.Where(commonKeyCol+" = ?", "dflop-upgrade-preserved").First(&preserved).Error)
			assert.Equal(t, "release-rc40", preserved.Value)
			assert.Error(t, DB.Create(&Option{Key: "dflop-upgrade-preserved", Value: "duplicate"}).Error)
			require.NoError(t, DB.Where(commonKeyCol+" = ?", "dflop-upgrade-preserved").First(&preserved).Error)
			assert.Equal(t, "release-rc40", preserved.Value)
			assert.True(t, DB.Migrator().HasTable(&PricingSyncRun{}))
			assert.True(t, DB.Migrator().HasTable(&PricingSyncItem{}))
			assert.True(t, DB.Migrator().HasTable(&PricingSyncManaged{}))
			assert.True(t, DB.Migrator().HasColumn(&PricingSyncItem{}, "pricing_shape"))
			assert.True(t, DB.Migrator().HasColumn(&PricingSyncItem{}, "reason_code"))
			assert.True(t, DB.Migrator().HasColumn(&PricingSyncManaged{}, "pricing_shape"))
		})
	}
}
