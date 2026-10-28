package postgres

import (
	"sync"

	"godeb/backend/db"
	"gorm.io/gorm"
)

// accountRow remembers DB users GoDeb created (password sealed).
type accountRow struct {
	Name     string `gorm:"primaryKey"`
	Password string `vault:"secret"`
	Created  string
}

func (accountRow) TableName() string { return "postgres_accounts" }

func init() { db.RegisterAgentSecret("postgres_accounts", "name", "password") }

var (
	migrateOnce sync.Once
	migrateErr  error
)

func store() (*gorm.DB, error) {
	gdb, err := db.AgentGorm()
	if err != nil {
		return nil, err
	}
	migrateOnce.Do(func() { migrateErr = gdb.AutoMigrate(&accountRow{}) })
	return gdb, migrateErr
}

func saveAccount(name, password, created string) error {
	gdb, err := store()
	if err != nil {
		return err
	}
	return gdb.Save(&accountRow{Name: name, Password: password, Created: created}).Error
}

func deleteAccount(name string) error {
	gdb, err := store()
	if err != nil {
		return err
	}
	return gdb.Where("name = ?", name).Delete(&accountRow{}).Error
}

func listAccounts() ([]accountRow, error) {
	gdb, err := store()
	if err != nil {
		return nil, err
	}
	var rows []accountRow
	if err := gdb.Order("name").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
