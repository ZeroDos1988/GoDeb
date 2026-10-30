package apphttps

import (
	"sync"

	"gorm.io/gorm"

	"godeb/backend/db"
)

// settingsRow lives in the agent database: the agent acts on it, so it is
// never read back from the state directory of the web process.
type settingsRow struct {
	ID         int `gorm:"primaryKey"`
	Label      string
	Zone       string
	Host       string
	Enabled    bool
	AccountKey string `vault:"secret"` // PEM, ACME account
	CertPEM    string // chain, public
	CertKey    string `vault:"secret"` // PEM
	NotAfter   string // RFC 3339
	LastError  string
	LastRun    string
}

func (settingsRow) TableName() string { return "apphttps_settings" }

func init() {
	db.RegisterAgentSecret("apphttps_settings", "id", "account_key")
	db.RegisterAgentSecret("apphttps_settings", "id", "cert_key")
}

var (
	migrateMu sync.Mutex
	migrated  = map[string]bool{}
	// rowMu serialises read-modify-write of the single row.
	rowMu sync.Mutex
)

func store() (*gorm.DB, error) {
	gdb, err := db.AgentGorm()
	if err != nil {
		return nil, err
	}
	path := db.AgentPath()
	migrateMu.Lock()
	defer migrateMu.Unlock()
	if !migrated[path] {
		if err := gdb.AutoMigrate(&settingsRow{}); err != nil {
			return nil, err
		}
		migrated[path] = true
	}
	return gdb, nil
}

func load() (settingsRow, error) {
	gdb, err := store()
	if err != nil {
		return settingsRow{}, err
	}
	var row settingsRow
	if err := gdb.Where("id = 1").Limit(1).Find(&row).Error; err != nil {
		return settingsRow{}, err
	}
	row.ID = 1
	return row, nil
}

// update changes the row under rowMu.
func update(fn func(*settingsRow)) (settingsRow, error) {
	rowMu.Lock()
	defer rowMu.Unlock()
	row, err := load()
	if err != nil {
		return settingsRow{}, err
	}
	fn(&row)
	gdb, err := store()
	if err != nil {
		return settingsRow{}, err
	}
	return row, gdb.Save(&row).Error
}
