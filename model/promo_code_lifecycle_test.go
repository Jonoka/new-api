package model

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func newLifecyclePromoCode(code string) *PromoCode {
	return &PromoCode{
		Name:           "lifecycle promo",
		Code:           code,
		Status:         common.RedemptionCodeStatusEnabled,
		DiscountType:   PromoCodeDiscountTypePercent,
		DiscountValue:  10,
		AppliesToTopup: true,
		MaxRedeemCount: 10,
	}
}

func TestPromoCodeDeleteAllowsCodeReuseAndKeepsHistory(t *testing.T) {
	truncateTables(t)

	original := newLifecyclePromoCode("REUSE_ME")
	require.NoError(t, original.Insert())
	require.NoError(t, DeletePromoCodeById(original.Id))

	recreated := newLifecyclePromoCode("reuse_me")
	require.NoError(t, recreated.Insert())
	assert.NotEqual(t, original.Id, recreated.Id)
	assert.Equal(t, "REUSE_ME", recreated.Code)

	var archived PromoCode
	require.NoError(t, DB.Unscoped().First(&archived, original.Id).Error)
	assert.True(t, archived.DeletedAt.Valid)
	assert.Equal(t, "REUSE_ME", archived.Code)
	assert.Equal(t, original.Id, archived.DeletedId)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return recordPromoCodeUsageTx(
			tx, original.Id, 901, PromoCodeTargetTopUp, "deleted-promo-order",
			100, 10, 90, false,
		)
	}))

	var usage PromoCodeUsage
	require.NoError(t, DB.Where("promo_code_id = ?", original.Id).First(&usage).Error)
	assert.Equal(t, "deleted-promo-order", usage.OrderNo)
	require.NoError(t, DB.Unscoped().First(&archived, original.Id).Error)
	assert.Equal(t, 1, archived.RedeemedCount)
	assert.True(t, archived.DeletedAt.Valid)
	var active PromoCode
	require.NoError(t, DB.First(&active, recreated.Id).Error)
	assert.Zero(t, active.RedeemedCount)

	require.NoError(t, DeletePromoCodeById(recreated.Id))
	third := newLifecyclePromoCode("REUSE_ME")
	require.NoError(t, third.Insert())
	assert.NotEqual(t, recreated.Id, third.Id)
	var secondArchived PromoCode
	require.NoError(t, DB.Unscoped().First(&secondArchived, recreated.Id).Error)
	assert.Equal(t, "REUSE_ME", secondArchived.Code)
	assert.Equal(t, recreated.Id, secondArchived.DeletedId)
}

func TestPromoCodeInsertRepairsLegacySoftDeletedCollision(t *testing.T) {
	truncateTables(t)

	legacy := newLifecyclePromoCode("LEGACY_REUSE")
	require.NoError(t, legacy.Insert())
	// 模拟旧版本只写 deleted_at、没有释放唯一 code 的记录。
	require.NoError(t, DB.Delete(&PromoCode{}, legacy.Id).Error)

	recreated := newLifecyclePromoCode("legacy_reuse")
	require.NoError(t, recreated.Insert())
	assert.Equal(t, "LEGACY_REUSE", recreated.Code)

	var archived PromoCode
	require.NoError(t, DB.Unscoped().First(&archived, legacy.Id).Error)
	assert.True(t, archived.DeletedAt.Valid)
	assert.Equal(t, "LEGACY_REUSE", archived.Code)
	assert.Equal(t, legacy.Id, archived.DeletedId)

	var activeCount int64
	require.NoError(t, DB.Model(&PromoCode{}).Where("code = ?", "LEGACY_REUSE").Count(&activeCount).Error)
	assert.EqualValues(t, 1, activeCount)
}

func TestPromoCodeInsertRejectsActiveDuplicate(t *testing.T) {
	truncateTables(t)

	require.NoError(t, newLifecyclePromoCode("ACTIVE_DUPLICATE").Insert())
	err := newLifecyclePromoCode("active_duplicate").Insert()
	require.EqualError(t, err, "优惠码已存在")

	var activeCount int64
	require.NoError(t, DB.Model(&PromoCode{}).Where("code = ?", "ACTIVE_DUPLICATE").Count(&activeCount).Error)
	assert.EqualValues(t, 1, activeCount)
}

func TestPromoCodeUpdateCanReuseLegacySoftDeletedCode(t *testing.T) {
	truncateTables(t)

	legacy := newLifecyclePromoCode("LEGACY_UPDATE")
	require.NoError(t, legacy.Insert())
	require.NoError(t, DB.Delete(&PromoCode{}, legacy.Id).Error)

	current := newLifecyclePromoCode("CURRENT_CODE")
	require.NoError(t, current.Insert())
	current.Code = "legacy_update"
	require.NoError(t, current.Update())
	assert.Equal(t, "LEGACY_UPDATE", current.Code)

	var archived PromoCode
	require.NoError(t, DB.Unscoped().First(&archived, legacy.Id).Error)
	assert.Equal(t, "LEGACY_UPDATE", archived.Code)
	assert.Equal(t, legacy.Id, archived.DeletedId)

	var saved PromoCode
	require.NoError(t, DB.First(&saved, current.Id).Error)
	assert.Equal(t, "LEGACY_UPDATE", saved.Code)
}

type legacyPromoCodeMigration struct {
	Id        int
	Code      string         `gorm:"type:varchar(64);uniqueIndex"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (legacyPromoCodeMigration) TableName() string {
	return "promo_codes"
}

func TestMigratePromoCodeDeletionKeyPreservesCodeAndReleasesUniqueValue(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&legacyPromoCodeMigration{}))

	legacy := &legacyPromoCodeMigration{Code: "MIGRATION_REUSE"}
	require.NoError(t, db.Create(legacy).Error)
	require.NoError(t, db.Delete(legacy).Error)

	require.NoError(t, db.AutoMigrate(&PromoCode{}))
	require.NoError(t, migratePromoCodeDeletionKey(db))
	require.NoError(t, migratePromoCodeDeletionKey(db))
	assert.True(t, db.Migrator().HasIndex(&PromoCode{}, promoCodeUniqueIndex))
	assert.False(t, db.Migrator().HasIndex(&PromoCode{}, promoCodeLegacyCodeIndex))

	var migrated PromoCode
	require.NoError(t, db.Unscoped().First(&migrated, legacy.Id).Error)
	assert.Equal(t, "MIGRATION_REUSE", migrated.Code)
	assert.Equal(t, legacy.Id, migrated.DeletedId)

	active := newLifecyclePromoCode("MIGRATION_REUSE")
	require.NoError(t, db.Create(active).Error)
	require.Error(t, db.Create(newLifecyclePromoCode("MIGRATION_REUSE")).Error)
}

func TestMigratePromoCodeDeletionKeyDropsPostgreSQLLegacyConstraint(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("NEW_API_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("NEW_API_TEST_POSTGRES_DSN is not configured")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	var databaseName string
	require.NoError(t, db.Raw("SELECT current_database()").Scan(&databaseName).Error)
	require.True(t, strings.HasPrefix(databaseName, "newapi_test_"), "refusing destructive migration test against database %q", databaseName)

	require.NoError(t, db.Migrator().DropTable(&PromoCode{}))
	t.Cleanup(func() {
		_ = db.Migrator().DropTable(&PromoCode{})
	})
	require.NoError(t, db.Exec(`
		CREATE TABLE promo_codes (
			id BIGSERIAL PRIMARY KEY,
			code VARCHAR(64),
			deleted_at TIMESTAMPTZ,
			CONSTRAINT idx_promo_codes_code UNIQUE (code)
		)
	`).Error)
	require.NoError(t, db.Exec("INSERT INTO promo_codes (code, deleted_at) VALUES (?, NOW())", "PG_MIGRATION_REUSE").Error)
	require.NoError(t, db.AutoMigrate(&PromoCode{}))
	require.True(t, db.Migrator().HasConstraint(&PromoCode{}, promoCodeLegacyCodeIndex))

	require.NoError(t, migratePromoCodeDeletionKey(db))
	require.NoError(t, migratePromoCodeDeletionKey(db))
	assert.False(t, db.Migrator().HasConstraint(&PromoCode{}, promoCodeLegacyCodeIndex))
	assert.False(t, db.Migrator().HasIndex(&PromoCode{}, promoCodeLegacyCodeIndex))
	assert.True(t, db.Migrator().HasIndex(&PromoCode{}, promoCodeUniqueIndex))

	var migrated PromoCode
	require.NoError(t, db.Unscoped().Where("code = ?", "PG_MIGRATION_REUSE").First(&migrated).Error)
	assert.Equal(t, migrated.Id, migrated.DeletedId)
	require.NoError(t, db.Create(newLifecyclePromoCode("PG_MIGRATION_REUSE")).Error)
	require.Error(t, db.Create(newLifecyclePromoCode("PG_MIGRATION_REUSE")).Error)
}
