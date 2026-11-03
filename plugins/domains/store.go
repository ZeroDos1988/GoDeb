package domains

import (
	"sync"
	"time"

	"gorm.io/gorm"

	"godeb/backend/db"
)

type settingsRow struct {
	ID       int    `gorm:"primaryKey"`
	Token    string `vault:"secret"`
	ServerID int64
}

func (settingsRow) TableName() string { return "domains_settings" }

func init() { db.RegisterAgentSecret("domains_settings", "id", "token") }

type domainRow struct {
	Name     string `gorm:"primaryKey"`
	Mode     string
	Created  string
	Defaults bool
}

func (r domainRow) domain() Domain {
	return Domain{Name: r.Name, Mode: r.Mode, Created: r.Created, Defaults: r.Defaults}
}

func (domainRow) TableName() string { return "domains_zones" }

var (
	migrateMu sync.Mutex
	migrated  = map[string]bool{}
)

// store migrates once per database file; tests switch files via env.
func store() (*gorm.DB, error) {
	gdb, err := db.AgentGorm()
	if err != nil {
		return nil, err
	}
	path := db.AgentPath()
	migrateMu.Lock()
	defer migrateMu.Unlock()
	if !migrated[path] {
		if err := gdb.AutoMigrate(&settingsRow{}, &domainRow{}); err != nil {
			return nil, err
		}
		migrated[path] = true
	}
	return gdb, nil
}

func loadSettings() (Settings, string, error) {
	gdb, err := store()
	if err != nil {
		return Settings{}, "", err
	}
	var row settingsRow
	res := gdb.Where("id = 1").Limit(1).Find(&row)
	if res.Error != nil {
		return Settings{}, "", res.Error
	}
	s := Settings{ServerID: row.ServerID}
	if row.Token != "" {
		s.TokenSet = true
		if n := len(row.Token); n >= 4 {
			s.TokenHint = row.Token[n-4:]
		}
	}
	return s, row.Token, nil
}

func upsertSettings(token *string, serverID *int64) error {
	gdb, err := store()
	if err != nil {
		return err
	}
	var row settingsRow
	_ = gdb.Where("id = 1").Limit(1).Find(&row)
	row.ID = 1
	if token != nil {
		row.Token = *token
	}
	if serverID != nil {
		row.ServerID = *serverID
	}
	return gdb.Save(&row).Error
}

func listDomains() ([]Domain, error) {
	gdb, err := store()
	if err != nil {
		return nil, err
	}
	var rows []domainRow
	if err := gdb.Order("name").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Domain, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.domain())
	}
	return out, nil
}

func getDomain(name string) (Domain, bool, error) {
	gdb, err := store()
	if err != nil {
		return Domain{}, false, err
	}
	var row domainRow
	res := gdb.Where("name = ?", name).Limit(1).Find(&row)
	if res.Error != nil {
		return Domain{}, false, res.Error
	}
	if res.RowsAffected == 0 {
		return Domain{}, false, nil
	}
	return row.domain(), true, nil
}

func addDomain(name, mode string, defaults bool) error {
	gdb, err := store()
	if err != nil {
		return err
	}
	return gdb.Create(&domainRow{Name: name, Mode: mode, Defaults: defaults, Created: time.Now().UTC().Format(time.RFC3339)}).Error
}

func updateDomain(name, mode string, defaults bool) error {
	gdb, err := store()
	if err != nil {
		return err
	}
	return gdb.Model(&domainRow{}).Where("name = ?", name).Updates(map[string]any{"mode": mode, "defaults": defaults}).Error
}

func deleteDomain(name string) error {
	gdb, err := store()
	if err != nil {
		return err
	}
	return gdb.Delete(&domainRow{}, "name = ?", name).Error
}
