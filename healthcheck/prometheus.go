package healthcheck

import (
	"context"
	"fmt"
	"time"

	"github.com/m-lab/rebot/node"
	"github.com/m-lab/rebot/promtest"
	"github.com/prometheus/common/model"
	log "github.com/sirupsen/logrus"
)

// CandidatesQuery is a Prometheus query to determine what machines need to
// be rebooted. A machine is to be rebooted if it's unreachable via SSH or
// it's taking too long to boot, unless it's in GMX, lame-duck or its whole
// site is currently offline.
var CandidatesQuery = `
	(
		# machine booted > 15m ago but hasn't reported success yet.
		# The label_replace is needed because the epoxy_* metrics lack "site".
		label_replace(
		epoxy_last_boot < time() - 900 and epoxy_last_success < epoxy_last_boot
		, "site", "$1", "machine", "mlab[1-4]-([a-z]{3}[0-9t]{2}).+") OR
		# machine has been unreachable over SSH for the past 15m and is not currently booting
		(sum_over_time(probe_success{service="ssh", module="ssh_v4_online"}[%[1]dm]) == 0
			unless on(machine) epoxy_last_boot > time() - 900)
	)
	# Exclude machines in GMX.
	unless on(machine) gmx_machine_maintenance == 1
	# Exclude lame-ducked Kubernetes nodes.
	unless on (machine) kube_node_spec_taint{key="lame-duck"} == 1
	# Exclude nodes where the site is in GMX.
	unless on(site) gmx_site_maintenance == 1
	# Exclude nodes where the switch is offline.
	unless on(site) sum_over_time(probe_success{instance=~"s1.*", module="icmp"}[%[1]dm]) == 0`

// GetOfflineNodes checks for offline nodes in the last N minutes.
// It returns a Vector of samples.
func GetOfflineNodes(prom promtest.PromClient, minutes int) ([]node.Node, error) {
	values, warnings, err := prom.Query(context.Background(), fmt.Sprintf(CandidatesQuery, minutes), time.Now())
	if warnings != nil {
		for warn := range warnings {
			log.Warn(warn)
		}
	}
	if err != nil {
		return nil, err
	}

	if len(values.(model.Vector)) != 0 {
		log.WithFields(log.Fields{"nodes": values}).Warn("Offline nodes found.")
	}

	candidates := make([]node.Node, 0)

	for _, sample := range values.(model.Vector) {
		site := sample.Metric["site"]
		machine := sample.Metric["machine"]
		log.Info("adding " + string(machine))
		candidates = append(candidates, node.Node{
			Name: string(machine),
			Site: string(site),
		})
	}

	return candidates, nil
}
