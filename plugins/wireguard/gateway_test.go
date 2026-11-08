package wireguard

import (
	"context"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"godeb/backend/core"
)

const gatewayPeerConf = "# GoDeb-Role: peer\n[Interface]\nPrivateKey = " + testPriv + "\nAddress = 10.8.0.2/32\n\n[Peer]\nPublicKey = " + testPub +
	"\nEndpoint = vpn.example.com:51820\nAllowedIPs = 10.8.0.0/24\nPersistentKeepalive = 25\n"

func fakeLAN(t *testing.T, dev string, lans ...core.LANAddress) {
	t.Helper()
	oldLAN, oldRoute := lanAddresses, defaultRoute
	t.Cleanup(func() { lanAddresses, defaultRoute = oldLAN, oldRoute })
	lanAddresses = func() []core.LANAddress { return lans }
	defaultRoute = func(context.Context) core.Route { return core.Route{Dev: dev, Src: "192.168.178.20"} }
}

func TestGatewayPlan(t *testing.T) {
	fakeLAN(t, "eth0", core.LANAddress{Iface: "eth0", Address: "192.168.178.20", Prefix: "192.168.178.0/24"})
	lan, tunnel, err := gatewayPlan(t.Context(), gatewayPeerConf)
	if err != nil {
		t.Fatal(err)
	}
	if lan.Iface != "eth0" || lan.Prefix != "192.168.178.0/24" || tunnel.String() != "10.8.0.0/24" {
		t.Fatalf("plan %+v %s", lan, tunnel)
	}
	if _, _, err := gatewayPlan(t.Context(), sampleConfig); err == nil {
		t.Fatal("gateway on a hub")
	}
	fakeLAN(t, "eth0", core.LANAddress{Iface: "eth0", Address: "10.8.0.50", Prefix: "10.8.0.0/16"})
	if _, _, err := gatewayPlan(t.Context(), gatewayPeerConf); err == nil {
		t.Fatal("LAN overlapping the tunnel accepted")
	}
	fakeLAN(t, "wlan0", core.LANAddress{Iface: "eth0", Address: "192.168.178.20", Prefix: "192.168.178.0/24"})
	if _, _, err := gatewayPlan(t.Context(), gatewayPeerConf); err == nil {
		t.Fatal("LAN without default route accepted")
	}
}

func TestGatewayNft(t *testing.T) {
	tunnel := netip.MustParsePrefix("10.8.0.0/24")
	on := renderGatewayNft(gatewayState{Enabled: true, LANIf: "eth0", LAN: "192.168.178.0/24"}, tunnel)
	if !strings.Contains(on, `iifname "eth0" oifname "wg0" ip saddr 192.168.178.0/24 ip daddr 10.8.0.0/24 masquerade`) {
		t.Fatalf("rule:\n%s", on)
	}
	off := renderGatewayNft(gatewayState{}, tunnel)
	if strings.Contains(off, "masquerade") || !strings.Contains(off, "delete table ip godeb_gateway") {
		t.Fatalf("off:\n%s", off)
	}
	// Check both with the real nft in a network namespace of our own.
	nft, err := exec.LookPath("nft")
	if err != nil {
		nft = "/usr/sbin/nft"
	}
	if _, err := os.Stat(nft); err != nil {
		t.Skip("nft fehlt")
	}
	for name, script := range map[string]string{"on": on, "off": off} {
		path := filepath.Join(t.TempDir(), name+".nft")
		if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command("unshare", "-rn", nft, "-c", "-f", path).CombinedOutput()
		if err != nil && strings.Contains(string(out), "Operation not permitted") {
			t.Skip("kein Netzwerk-Namespace ohne root")
		}
		if err != nil {
			t.Fatalf("%s: nft -c: %v\n%s", name, err, out)
		}
	}
}
