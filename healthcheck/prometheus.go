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

var (
	// NodeQuery is a Prometheus query to determine what nodes aren't reachable
	// on any of the currently used SSH ports (22 or 806). Additional filtering
	// based on gmx maintenance, lame-duck mode, or the presence of recent NDT
	// tests on the node is applied. This makes sure we never reboot a node
	// unnecessarily or lose data.
	NodeQuery = `sum_over_time(probe_success{service="ssh", module="ssh_v4_online"}[%[1]dm]) == 0
			unless on (machine)
				sum_over_time(probe_success{service="ssh806", module="ssh_v4_online"}[%[1]dm]) > 0
			unless on(machine) gmx_machine_maintenance == 1
			unless on(site) gmx_site_maintenance == 1
			unless on (machine) lame_duck_node == 1
			unless on (machine) kube_node_spec_taint{key="lame-duck"} == 1
			unless on (machine) count_over_time(probe_success{service="ssh", module="ssh_v4_online"}[%[1]dm]) < 14
			unless on (machine) rate(inotify_extension_create_total{ext=".s2c_snaplog"}[%[1]dm]) > 0
			unless on (machine) increase(ndt_test_total[%[1]dm]) > 0`

	// SwitchQuery is a prometheus query to determine what switches are
	// offline.  To determine if a switch is offline, pings are generally
	// more reliable than SNMP scraping.
	SwitchQuery = `sum_over_time(probe_success{instance=~"s1.*", module="icmp"}[15m]) == 0 unless on(site) gmx_site_maintenance == 1`
)

// getOfflineSites checks for offline switches in the last N minutes.
// It returns a sitename -> Sample map.
func getOfflineSites(prom promtest.PromClient) (map[string]*model.Sample, error) {
	offline := make(map[string]*model.Sample)

	values, warnings, err := prom.Query(context.Background(), SwitchQuery, time.Now())
	if warnings != nil {
		for warn := range warnings {
			log.Warn(warn)
		}
	}
	if err != nil {
		return nil, err
	}

	for _, s := range values.(model.Vector) {
		offline[string(s.Metric["site"])] = s
		log.WithFields(log.Fields{"site": s.Metric["site"]}).Warn("Offline switch found.")
	}

	return offline, err
}

// getOfflineNodes checks for offline nodes in the last N minutes.
// It returns a Vector of samples.
func getOfflineNodes(prom promtest.PromClient, minutes int) ([]node.Node, error) {
	values, warnings, err := prom.Query(context.Background(), fmt.Sprintf(NodeQuery, minutes), time.Now())
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

func filterOfflineSites(sites map[string]*model.Sample, toFilter []node.Node) []node.Node {

	filtered := make([]node.Node, 0)

	for _, c := range toFilter {
		// Ignore machines in sites where the switch is offline.
		site := c.Site
		machine := c.Name
		if _, ok := sites[site]; !ok {
			filtered = append(filtered, c)
		} else {
			log.Info("Ignoring " + machine + " as the switch is offline.")
		}
	}

	return filtered
}

// GetRebootable makes a list of nodes that are candidates for being rebooted.
// To do so, it queries both offline nodes and offline sites and returns
// offline nodes that are not part of offline sites.
func GetRebootable(prom promtest.PromClient, minutes int) ([]node.Node, error) {
	// Query for offline switches
	sites, err := getOfflineSites(prom)
	if err != nil {
		log.Error("Unable to retrieve offline sites from Prometheus")
		return nil, err
	}

	// Query for offline nodes
	nodes, err := getOfflineNodes(prom, minutes)
	if err != nil {
		log.Error("Unable to retrieve offline nodes from Prometheus")
		return nil, err
	}

	offline := filterOfflineSites(sites, nodes)
	return offline, nil
}
