package portforward

import (
	"context"

	"godeb/backend/metrics"
)

func init() {
	metrics.RegisterPorts("portforward", forwardPorts)
}

// forwardPorts lets the monitor count the traffic the hub forwards per rule.
func forwardPorts(ctx context.Context) []metrics.Port {
	rules, err := loadRules()
	if err != nil {
		return nil
	}
	var out []metrics.Port
	for _, r := range rules {
		var ports []int
		for p := r.From; p <= r.To && p-r.From < maxRange; p++ {
			ports = append(ports, p)
		}
		out = append(out, metrics.Port{Name: "fwd-" + r.Name, Label: r.Name + " (Weiterleitung)", Proto: r.Proto, Ports: ports, Forward: true})
	}
	return out
}
