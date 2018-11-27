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
	nodeQuery = `(label_replace(sum_over_time(probe_success{service="ssh806", module="ssh_v4_online"}[%[1]dm]) == 0,
	"site", "$1", "machine", ".+?\\.(.+?)\\..+")
	unless on (machine)
	label_replace(sum_over_time(probe_success{service="ssh", module="ssh_v4_online"}[%[1]dm]) > 0,
	"site", "$1", "machine", ".+?\\.(.+?)\\..+"))
			unless on(machine) gmx_machine_maintenance == 1
			unless on(site) gmx_site_maintenance == 1
			unless on (machine) lame_duck_node == 1
			unless on (machine) count_over_time(probe_success{service="ssh806", module="ssh_v4_online"}[%[1]dm]) < 14
			unless on (machine) rate(inotify_extension_create_total{ext=".s2c_snaplog"}[%[1]dm]) > 0`

	// To determine if a switch is offline, pings are generally more reliable
	// than SNMP scraping.
	switchQuery = `sum_over_time(probe_success{instance=~"s1.*", module="icmp"}[15m]) == 0 unless on(site) gmx_site_maintenance == 1`
)

// GetOfflineSites checks for offline switches in the last N minutes.
// It returns a sitename -> Sample map.
func GetOfflineSites(prom promtest.PromClient) (map[string]*model.Sample, error) {
	offline := make(map[string]*model.Sample)

	values, err := prom.Query(context.Background(), switchQuery, time.Now())
	if err != nil {
		return nil, err
	}

	for _, s := range values.(model.Vector) {
		offline[string(s.Metric["site"])] = s
		log.WithFields(log.Fields{"site": s.Metric["site"]}).Warn("Offline switch found.")
	}

	return offline, err
}

// GetOfflineNodes checks for offline nodes in the last N minutes.
// It returns a Vector of samples.
func GetOfflineNodes(prom promtest.PromClient, minutes int) ([]node.Node, error) {
	values, err := prom.Query(context.Background(), fmt.Sprintf(nodeQuery, minutes), time.Now())
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

func FilterOfflineSites(sites map[string]*model.Sample, toFilter []node.Node) []node.Node {

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
