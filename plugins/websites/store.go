package websites

import (
	"errors"
	"sync"

	"godeb/backend/db"
	"gorm.io/gorm"
)

var errNotFound = errors.New("Website gibt es nicht")

type siteRow struct {
	Name     string `gorm:"primaryKey"`
	Domain   string
	Mode     string
	PHP      bool
	ProxyURL string
	TLS      bool
	Public   bool
	Enabled  bool
	Created  string
}

func (siteRow) TableName() string { return "websites_sites" }

var (
	migrateOnce sync.Once
	migrateErr  error
)

func store() (*gorm.DB, error) {
	gdb, err := db.AgentGorm()
	if err != nil {
		return nil, err
	}
	migrateOnce.Do(func() { migrateErr = gdb.AutoMigrate(&siteRow{}) })
	return gdb, migrateErr
}

func loadSites() ([]Site, error) {
	gdb, err := store()
	if err != nil {
		return nil, err
	}
	var rows []siteRow
	if err := gdb.Order("name").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Site, 0, len(rows))
	for _, r := range rows {
		s, err := cleanSite(Site{
			Name: r.Name, Domain: r.Domain, Mode: r.Mode, PHP: r.PHP,
			ProxyURL: r.ProxyURL, TLS: r.TLS, Public: r.Public, Enabled: r.Enabled, Created: r.Created,
		})
		if err != nil {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

func saveSite(s Site) error {
	gdb, err := store()
	if err != nil {
		return err
	}
	return gdb.Save(&siteRow{
		Name: s.Name, Domain: s.Domain, Mode: s.Mode, PHP: s.PHP,
		ProxyURL: s.ProxyURL, TLS: s.TLS, Public: s.Public, Enabled: s.Enabled, Created: s.Created,
	}).Error
}

func deleteSiteRow(name string) error {
	gdb, err := store()
	if err != nil {
		return err
	}
	return gdb.Where("name = ?", name).Delete(&siteRow{}).Error
}

func findSite(name string) (Site, error) {
	list, err := loadSites()
	if err != nil {
		return Site{}, err
	}
	for _, s := range list {
		if s.Name == name {
			return s, nil
		}
	}
	return Site{}, errNotFound
}
