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
func getOfflineNodes(prom promClient, minutes int) (model.Vector, error) {

	values, err := prom.Query(context.Background(), fmt.Sprintf(nodeQuery, minutes), time.Now())
	if err != nil {
		return nil, err
	}

	if len(values.(model.Vector)) != 0 {
		log.WithFields(log.Fields{"nodes": values}).Warn("Offline nodes found.")
	}

	return values.(model.Vector), err
}

func filterOfflineSites(sites map[string]*model.Sample, nodes model.Vector) []string {
	var candidates []string

	for _, value := range nodes {
		// Ignore machines in sites where the switch is offline.
		site := string(value.Metric["site"])
		machine := string(value.Metric["machine"])
		if _, ok := sites[site]; !ok {
			candidates = append(candidates, string(value.Metric["machine"]))
		} else {
			log.Info("Ignoring " + machine + " as the switch is offline.")
		}
	}

	return candidates
}

// filterRecent filters out nodes that were rebooted less than 24 hours ago.
func filterRecent(nodes []string, candidateHistory map[string]candidate) []string {
	var rebootList []string

	for _, node := range nodes {
		candidate, ok := candidateHistory[node]
		if ok {
			// This candidate has been down before.
			// Check to see if the previous time was w/in the past 24 hours
			if time.Now().Sub(candidate.LastReboot) > 24*time.Hour {
				rebootList = append(rebootList, candidate.Name)
			} else {
				log.WithFields(log.Fields{"node": candidate.Name, "LastReboot": candidate.LastReboot}).Info("The node was rebooted recently - skipping it.")
			}
		} else {
			// New candidate - just add it to the list.
			rebootList = append(rebootList, node)
		}
	}

	return rebootList
}
