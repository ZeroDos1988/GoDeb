package mail

import (
	"encoding/json"
	"errors"
	"sync"

	"gorm.io/gorm"

	"godeb/backend/db"
)

// The metadata lives in the agent database (/etc/godeb, root). The agent
// acts on it, so it must not come from the web process's data.

type settingsRow struct {
	ID       int `gorm:"primaryKey"`
	Hostname string
	ClamAV   bool
	CertFile string
	KeyFile  string
}

func (settingsRow) TableName() string { return "mailserver_settings" }

type domainRow struct {
	Name       string `gorm:"primaryKey"`
	Selector   string
	DKIMPublic string
	CatchAll   string
	Suspended  bool
	Created    string
}

func (domainRow) TableName() string { return "mailserver_domains" }

type accountRow struct {
	Address     string `gorm:"primaryKey"`
	Domain      string `gorm:"index"`
	Local       string
	Hash        string `vault:"secret"`
	QuotaMB     int
	Aliases     string // JSON list
	Forward     string // JSON list
	ForwardOnly bool
	Suspended   bool
	Created     string
}

func (accountRow) TableName() string { return "mailserver_accounts" }

var (
	migrateOnce sync.Once
	migrateErr  error
)

func store() (*gorm.DB, error) {
	gdb, err := db.AgentGorm()
	if err != nil {
		return nil, err
	}
	migrateOnce.Do(func() { migrateErr = gdb.AutoMigrate(&settingsRow{}, &domainRow{}, &accountRow{}) })
	return gdb, migrateErr
}

// loadSettings returns the settings; ok is false before the installation.
func loadSettings() (Settings, bool, error) {
	gdb, err := store()
	if err != nil {
		return Settings{}, false, err
	}
	var row settingsRow
	res := gdb.Where("id = 1").Limit(1).Find(&row)
	if res.Error != nil {
		return Settings{}, false, res.Error
	}
	if res.RowsAffected == 0 || row.Hostname == "" {
		return Settings{CertFile: snakeoilCert, KeyFile: snakeoilKey}, false, nil
	}
	s := Settings{Hostname: row.Hostname, ClamAV: row.ClamAV, CertFile: row.CertFile, KeyFile: row.KeyFile}
	if s.CertFile == "" || s.KeyFile == "" {
		s.CertFile, s.KeyFile = snakeoilCert, snakeoilKey
	}
	return s, true, nil
}

func saveSettings(s Settings) error {
	gdb, err := store()
	if err != nil {
		return err
	}
	return gdb.Save(&settingsRow{ID: 1, Hostname: s.Hostname, ClamAV: s.ClamAV, CertFile: s.CertFile, KeyFile: s.KeyFile}).Error
}

func loadAll() ([]Domain, []Account, error) {
	gdb, err := store()
	if err != nil {
		return nil, nil, err
	}
	var drows []domainRow
	if err := gdb.Order("name").Find(&drows).Error; err != nil {
		return nil, nil, err
	}
	var arows []accountRow
	if err := gdb.Order("address").Find(&arows).Error; err != nil {
		return nil, nil, err
	}
	domains := make([]Domain, 0, len(drows))
	for _, r := range drows {
		domains = append(domains, Domain{Name: r.Name, Selector: r.Selector, DKIMPublic: r.DKIMPublic, CatchAll: r.CatchAll, Suspended: r.Suspended, Created: r.Created})
	}
	accounts := make([]Account, 0, len(arows))
	for _, r := range arows {
		accounts = append(accounts, fromRow(r))
	}
	return domains, accounts, nil
}

func fromRow(r accountRow) Account {
	a := Account{Domain: r.Domain, Local: r.Local, Hash: r.Hash, QuotaMB: r.QuotaMB, ForwardOnly: r.ForwardOnly, Suspended: r.Suspended, Created: r.Created, Aliases: []string{}, Forward: []string{}}
	_ = json.Unmarshal([]byte(r.Aliases), &a.Aliases)
	_ = json.Unmarshal([]byte(r.Forward), &a.Forward)
	if a.Aliases == nil {
		a.Aliases = []string{}
	}
	if a.Forward == nil {
		a.Forward = []string{}
	}
	return a
}

func toRow(a Account) accountRow {
	al, _ := json.Marshal(a.Aliases)
	fw, _ := json.Marshal(a.Forward)
	return accountRow{Address: a.Address(), Domain: a.Domain, Local: a.Local, Hash: a.Hash, QuotaMB: a.QuotaMB, Aliases: string(al), Forward: string(fw), ForwardOnly: a.ForwardOnly, Suspended: a.Suspended, Created: a.Created}
}

func saveDomain(d Domain) error {
	gdb, err := store()
	if err != nil {
		return err
	}
	return gdb.Save(&domainRow{Name: d.Name, Selector: d.Selector, DKIMPublic: d.DKIMPublic, CatchAll: d.CatchAll, Suspended: d.Suspended, Created: d.Created}).Error
}

func deleteDomain(name string) error {
	gdb, err := store()
	if err != nil {
		return err
	}
	return gdb.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("domain = ?", name).Delete(&accountRow{}).Error; err != nil {
			return err
		}
		return tx.Where("name = ?", name).Delete(&domainRow{}).Error
	})
}

func saveAccount(a Account) error {
	gdb, err := store()
	if err != nil {
		return err
	}
	row := toRow(a)
	return gdb.Save(&row).Error
}

func deleteAccount(address string) error {
	gdb, err := store()
	if err != nil {
		return err
	}
	return gdb.Where("address = ?", address).Delete(&accountRow{}).Error
}

var errNotFound = errors.New("nicht gefunden")

func findDomain(domains []Domain, name string) (Domain, error) {
	for _, d := range domains {
		if d.Name == name {
			return d, nil
		}
	}
	return Domain{}, errNotFound
}

func findAccount(accounts []Account, address string) (Account, error) {
	for _, a := range accounts {
		if a.Address() == address {
			return a, nil
		}
	}
	return Account{}, errNotFound
}

// gatewayDomains counts the relay domains of the mail-gateway plugin.
// Both plugins configure Postfix; only one may be in use.
func gatewayDomains() int {
	gdb, err := store()
	if err != nil || !gdb.Migrator().HasTable("mail_domains") {
		return 0
	}
	var n int64
	if gdb.Table("mail_domains").Count(&n).Error != nil {
		return 0
	}
	return int(n)
}
