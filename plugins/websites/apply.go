package websites

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"godeb/backend/core"
)

func applySite(ctx context.Context, s Site) error {
	if err := ensureDocRoot(s); err != nil {
		return err
	}
	prevConf, prevErr := os.ReadFile(confPath(s.Name))
	_, linkErr := os.Lstat(enabledPath(s.Name))
	prevEnabled := linkErr == nil
	if err := writeConf(s); err != nil {
		return err
	}
	if err := setEnabled(s.Name, s.Enabled); err != nil {
		return err
	}
	if res := core.RunAdmin(ctx, "nginx", "-t"); res.Err != nil {
		// Never leave a broken vhost behind: the next reload of any site
		// would fail. Restore the previous state of this site.
		if prevErr == nil {
			_ = atomicWrite(confPath(s.Name), string(prevConf))
		} else {
			_ = os.Remove(confPath(s.Name))
			prevEnabled = false
		}
		_ = setEnabled(s.Name, prevEnabled)
		return fmt.Errorf("nginx -t: %s", res.Detail())
	}
	if res := core.RunAdminTimeout(ctx, time.Minute, "systemctl", "reload", "nginx"); res.Err != nil {
		return fmt.Errorf("nginx reload: %s", res.Detail())
	}
	if s.Public {
		_ = core.RunAdmin(ctx, "ufw", "--force", "allow", "80/tcp")
		_ = core.RunAdmin(ctx, "ufw", "--force", "allow", "443/tcp")
	}
	return nil
}

func setEnabled(name string, enabled bool) error {
	_ = os.Remove(enabledPath(name))
	if !enabled {
		return nil
	}
	return os.Symlink(confPath(name), enabledPath(name))
}

func removeSite(ctx context.Context, s Site) error {
	_ = os.Remove(enabledPath(s.Name))
	_ = os.Remove(confPath(s.Name))
	_ = os.RemoveAll(s.Root)
	_ = core.RunAdmin(ctx, "nginx", "-t")
	_ = core.RunAdminTimeout(ctx, time.Minute, "systemctl", "reload", "nginx")
	return nil
}

func writeConf(s Site) error {
	if err := os.MkdirAll(nginxAvail, 0o755); err != nil {
		return err
	}
	return atomicWrite(confPath(s.Name), renderNginx(s))
}

func atomicWrite(path, body string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".godeb-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func ensureDocRoot(s Site) error {
	if s.Mode != ModeStatic {
		return nil
	}
	if err := os.MkdirAll(wwwRoot, 0o755); err != nil {
		return err
	}
	return createDocRoot(s.Root, s.Domain)
}

// createDocRoot creates dir with a placeholder index.html, both owned by
// root. An existing directory is left alone: its contents belong to whoever
// maintains the site, and the agent never follows links inside it (no
// recursive chown, no write through a planted index.html symlink).
func createDocRoot(dir, domain string) error {
	fi, err := os.Lstat(dir)
	if err == nil {
		if !fi.IsDir() {
			return fmt.Errorf("%s ist kein Verzeichnis", dir)
		}
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return err
	}
	r, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer r.Close()
	f, err := r.OpenFile("index.html", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	body := fmt.Sprintf("<!doctype html><title>%s</title><h1>%s</h1><p>GoDeb website.</p>\n", domain, domain)
	if _, err := f.WriteString(body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func issueCert(ctx context.Context, domain, email string) error {
	if email != "" && !emailPattern.MatchString(email) {
		return errors.New("E-Mail-Adresse ist ungültig")
	}
	args := []string{"certonly", "--nginx", "--non-interactive", "--agree-tos", "-d", domain}
	if email != "" {
		args = append(args, "-m", email)
	} else {
		args = append(args, "--register-unsafely-without-email")
	}
	res := core.RunAdminTimeout(ctx, 5*time.Minute, "certbot", args...)
	if res.Err != nil {
		return fmt.Errorf("certbot: %s", res.Detail())
	}
	return nil
}
