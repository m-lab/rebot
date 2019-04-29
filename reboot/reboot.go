package reboot

import (
	"net/http"
	"time"

	"github.com/m-lab/rebot/node"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
)

const (
	// Default timeout for reboot requests. This is intentionally quite long to
	// accommodate for nodes that are slow to respond.
	clientTimeout = 2 * time.Minute
	rebootURL     = "/v1/reboot"
)

var (
	metricRebootRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rebot_reboot_requests_total",
			Help: "Total number of reboot requests performed.",
		},
		[]string{
			"machine",
			"site",
			"type",
			"status",
		},
	)
)

// ClientConfig holds the configuration for the Reboot API client.
type ClientConfig struct {
	Addr     string
	Username string
	Password string
}

// one reboots a single machine by send an HTTP request to the Reboot API
// and returns an error if the response code is not 200 or there is a timeout.
func one(client *http.Client, config *ClientConfig, toReboot node.Node) error {
	rebootURL := config.Addr + rebootURL + "?host" + toReboot.Name

	// Create the HTTP request
	request, err := http.NewRequest(http.MethodPost, rebootURL, nil)
	if err != nil {
		log.WithError(err).Error("Cannot create HTTP request.")
		return err
	}

	response, err := client.Do(request)
	if err != nil {
		log.Error(err)
		metricRebootRequests.WithLabelValues(toReboot.Name, toReboot.Site, "reboot", "failure").Add(1)
		return err
	}

	metricRebootRequests.WithLabelValues(toReboot.Name, toReboot.Site, "reboot", "success").Add(1)
	log.WithFields(log.Fields{"node": toReboot}).Info(response.Body)
	return nil
}

// Many reboots an array of machines and returns a map of
// machineName -> error for each element for which the rebootMany failed.
func Many(config *ClientConfig, toReboot []node.Node) map[string]error {
	errors := make(map[string]error)

	if len(toReboot) == 0 {
		log.Info("There are no nodes to reboot.")
		return errors
	}

	// Create the Reboot API client
	rebootClient := &http.Client{
		Timeout: clientTimeout,
	}

	// If there are more than 5 nodes to be rebooted, do nothing.
	// TODO(roberto) find a better way to report this case to the caller.
	if len(toReboot) > 5 {
		log.WithFields(log.Fields{"nodes": toReboot}).Error("There are more than 5 nodes offline, skipping.")
		return errors
	}

	log.WithFields(log.Fields{"nodes": toReboot}).Info("These nodes are going to be rebooted.")

	for _, c := range toReboot {
		log.WithFields(log.Fields{"node": c}).Info("Rebooting node...")
		err := one(rebootClient, config, c)
		if err != nil {
			errors[c.Name] = err
		}
	}

	return errors
}
