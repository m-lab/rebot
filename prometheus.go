package main

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/common/model"
	log "github.com/sirupsen/logrus"
)

// Prometheus HTTP client's interface
type promClient interface {
	Query(context.Context, string, time.Time) (model.Value, error)
}

// getOfflineSites checks for offline sites (switches) in the last N minutes.
// It returns a sitename -> Sample map.
func getOfflineSites(prom promClient) (map[string]*model.Sample, error) {
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

// getOfflineNodes checks for offline nodes in the last N minutes.
// It returns a Vector of samples.
func getOfflineNodes(prom promClient, minutes int) ([]candidate, error) {
	values, err := prom.Query(context.Background(), fmt.Sprintf(nodeQuery, minutes), time.Now())
	if err != nil {
		return nil, err
	}

	if len(values.(model.Vector)) != 0 {
		log.WithFields(log.Fields{"nodes": values}).Warn("Offline nodes found.")
	}

	candidates := make([]candidate, 0)

	for _, sample := range values.(model.Vector) {
		site := sample.Metric["site"]
		machine := sample.Metric["machine"]
		log.Info("adding " + string(machine))
		candidates = append(candidates, candidate{
			Name: string(machine),
			Site: string(site),
		})
	}

	return candidates, nil
}

func filterOfflineSites(sites map[string]*model.Sample, candidates []candidate) []candidate {

	filtered := make([]candidate, 0)

	for _, c := range candidates {
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

// filterRecent filters out nodes that were rebooted less than 24 hours ago.
func filterRecent(candidates []candidate, candidateHistory map[string]candidate) []candidate {
	filtered := make([]candidate, 0)

	for _, candidate := range candidates {
		history, ok := candidateHistory[candidate.Name]
		if ok {
			// This candidate has been down before.
			// Check to see if the previous time was w/in the past 24 hours
			if time.Now().Sub(history.LastReboot) > 24*time.Hour {
				filtered = append(filtered, candidate)
			} else {
				log.WithFields(log.Fields{"node": candidate.Name, "LastReboot": candidate.LastReboot}).Info("The node was rebooted recently - skipping it.")
			}
		} else {
			// New candidate - just add it to the list.
			filtered = append(filtered, candidate)
		}
	}

	return filtered
}
