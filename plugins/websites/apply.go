package websites

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"godeb/backend/core"
	"godeb/backend/state"
)

func applySite(ctx context.Context, s Site) error {
	if err := ensureDocRoot(s); err != nil {
		return err
	}
	if err := writeConf(s); err != nil {
		return err
	}
	if s.Enabled {
		_ = os.Remove(enabledPath(s.Name))
		if err := os.Symlink(confPath(s.Name), enabledPath(s.Name)); err != nil {
			return err
		}
	} else {
		_ = os.Remove(enabledPath(s.Name))
	}
	if err := linkHome(s); err != nil {
		return err
	}
	if res := core.RunAdmin(ctx, "nginx", "-t"); res.Err != nil {
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

func removeSite(ctx context.Context, s Site) error {
	_ = os.Remove(enabledPath(s.Name))
	_ = os.Remove(confPath(s.Name))
	_ = os.RemoveAll(s.Root)
	_ = unlinkHome(s)
	_ = core.RunAdmin(ctx, "nginx", "-t")
	_ = core.RunAdminTimeout(ctx, time.Minute, "systemctl", "reload", "nginx")
	return nil
}

func writeConf(s Site) error {
	if err := os.MkdirAll(nginxAvail, 0o755); err != nil {
		return err
	}
	body := renderNginx(s)
	tmp, err := os.CreateTemp(nginxAvail, ".godeb-*")
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
	return os.Rename(tmp.Name(), confPath(s.Name))
}

func ensureDocRoot(s Site) error {
	if s.Mode != ModeStatic {
		return nil
	}
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return err
	}
	index := filepath.Join(s.Root, "index.html")
	if _, err := os.Stat(index); os.IsNotExist(err) {
		body := fmt.Sprintf("<!doctype html><title>%s</title><h1>%s</h1><p>GoDeb website.</p>\n", s.Domain, s.Domain)
		if err := os.WriteFile(index, []byte(body), 0o644); err != nil {
			return err
		}
	}
	web := state.WebUser()
	if u, err := user.Lookup(web); err == nil && os.Geteuid() == 0 {
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		_ = os.Chown(s.Root, uid, gid)
		_ = filepath.Walk(s.Root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			_ = os.Chown(path, uid, gid)
			return nil
		})
	}
	return nil
}

func linkHome(s Site) error {
	web := state.WebUser()
	u, err := user.Lookup(web)
	if err != nil {
		return nil // no web user in dev
	}
	homeWWW := filepath.Join(u.HomeDir, homeWebRel)
	if err := os.MkdirAll(homeWWW, 0o755); err != nil {
		return err
	}
	link := filepath.Join(homeWWW, s.Name)
	_ = os.Remove(link)
	if s.Mode != ModeStatic {
		return nil
	}
	if err := os.Symlink(s.Root, link); err != nil {
		return err
	}
	if os.Geteuid() == 0 {
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		_ = os.Chown(homeWWW, uid, gid)
	}
	return nil
}

func unlinkHome(s Site) error {
	web := state.WebUser()
	u, err := user.Lookup(web)
	if err != nil {
		return nil
	}
	link := filepath.Join(u.HomeDir, homeWebRel, s.Name)
	if target, err := os.Readlink(link); err == nil && strings.HasPrefix(filepath.Clean(target), wwwRoot) {
		_ = os.Remove(link)
	}
	return nil
}

func issueCert(ctx context.Context, domain, email string) error {
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
