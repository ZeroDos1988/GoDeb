package pluginmgr

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"godeb/backend/core"
)

func TestReadAllValidatesContracts(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "ufw", `{
		"id": "ufw",
		"name": "UFW",
		"apt_dependencies": ["ufw"],
		"systemd_services": ["ufw.service"]
	}`)
	writePlugin(t, dir, "apt", `{"id":"apt","name":"APT","apt_dependencies":[],"systemd_services":[]}`)
	if err := os.MkdirAll(filepath.Join(dir, "skip"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := ReadAll(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "apt" || got[1].ID != "ufw" {
		t.Fatalf("got = %+v", got)
	}
	if _, err := Load(dir, "../ufw"); err == nil {
		t.Fatal("traversal id was accepted")
	}
	writePlugin(t, dir, "bad", `{"id":"bad","name":"Bad","apt_dependencies":["UF W"]}`)
	if _, err := Load(dir, "bad"); err == nil {
		t.Fatal("invalid package name was accepted")
	}
}

func TestInstallEnableDisableUsesDeclaredUnitsOnly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GODEB_DB", filepath.Join(dir, "godeb.db"))
	t.Setenv("GODEB_MASTER_KEY", filepath.Join(dir, "master.key"))
	writePlugin(t, dir, "fail2ban", `{
		"id": "fail2ban",
		"name": "Fail2ban",
		"apt_dependencies": ["fail2ban"],
		"systemd_services": ["fail2ban.service"]
	}`)

	var calls []string
	execute = func(ctx context.Context, timeout time.Duration, name string, args ...string) core.CommandResult {
		calls = append(calls, name+" "+stringsJoin(args))
		joined := stringsJoin(args)
		switch {
		case name == "dpkg-query":
			if stringsContains(joined, "fail2ban") && len(calls) > 2 {
				return core.CommandResult{Stdout: "installed"}
			}
			return core.CommandResult{Stdout: "not-installed"}
		case name == "systemctl" && len(args) > 0 && args[0] == "is-enabled":
			if stringsContains(joined, "fail2ban") && contains(calls, "systemctl enable --now fail2ban.service") {
				return core.CommandResult{Stdout: "enabled"}
			}
			return core.CommandResult{Stdout: "disabled"}
		case name == "systemctl" && len(args) > 0 && args[0] == "is-active":
			return core.CommandResult{Stdout: "inactive"}
		default:
			return core.CommandResult{Stdout: "ok"}
		}
	}
	t.Cleanup(func() { execute = func(ctx context.Context, timeout time.Duration, name string, args ...string) core.CommandResult {
		return core.RunAdminTimeout(ctx, timeout, name, args...)
	} })

	before, err := Scan(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if before[0].Installed || before[0].DependenciesReady {
		t.Fatalf("before = %+v", before[0])
	}
	desc, err := Load(dir, "fail2ban")
	if err != nil {
		t.Fatal(err)
	}
	if err := install(context.Background(), dir, desc); err != nil {
		t.Fatal(err)
	}
	if !contains(calls, "apt-get install -y --no-install-recommends fail2ban") {
		t.Fatalf("calls = %v", calls)
	}
	if err := enable(context.Background(), dir, desc); err != nil {
		t.Fatal(err)
	}
	if !contains(calls, "systemctl enable --now fail2ban.service") {
		t.Fatalf("calls = %v", calls)
	}
	after, err := Scan(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !after[0].Installed || !after[0].Enabled {
		t.Fatalf("after enable = %+v", after[0])
	}
	if err := disable(context.Background(), dir, desc); err != nil {
		t.Fatal(err)
	}
	if !contains(calls, "systemctl disable --now fail2ban.service") {
		t.Fatalf("calls = %v", calls)
	}
	off, err := Scan(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if off[0].Enabled || !IsDisabled("fail2ban") {
		t.Fatalf("after disable = %+v, IsDisabled=%v", off[0], IsDisabled("fail2ban"))
	}
	if !off[0].Installed {
		t.Fatal("disabling must not forget the installation")
	}
	if IsDisabled("apt") {
		t.Fatal("plugins without a state row must count as active")
	}
	if err := enable(context.Background(), dir, desc); err != nil {
		t.Fatal(err)
	}
	if IsDisabled("fail2ban") {
		t.Fatal("re-enabled plugin still disabled")
	}
}

func writePlugin(t *testing.T, dir, id, body string) {
	t.Helper()
	path := filepath.Join(dir, id)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "plugin.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func stringsJoin(args []string) string {
	out := ""
	for i, arg := range args {
		if i > 0 {
			out += " "
		}
		out += arg
	}
	return out
}

func stringsContains(hay, needle string) bool {
	return len(needle) == 0 || (len(hay) >= len(needle) && (hay == needle || containsString(hay, needle)))
}

func containsString(hay, needle string) bool {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
