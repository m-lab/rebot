package reboot

import (
	"errors"
	"io/ioutil"
	"net/http"

	"github.com/m-lab/rebot/node"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
)

// Endpoint for reboot requests, relative to the Reboot API's root.
const rebootEndpoint = "/v1/reboot"

var (
	metricRebootRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rebot_reboot_requests_total",
			Help: "Total number of reboot requests sent and " +
				"corresponding status.",
		},
		[]string{
			"machine",
			"site",
			"type",
			"status",
		},
	)

	// These can be swapped to simplify unit testing.
	newHTTPRequest = http.NewRequest
	readAll        = ioutil.ReadAll
	clientDo       = func(r *HTTPRebooter, req *http.Request) (*http.Response, error) {
		return r.client.Do(req)
	}
)

// HTTPRebooter reboots one of more nodes calling the Reboot API via the
// provided http.Client.
type HTTPRebooter struct {
	client   *http.Client
	baseURL  string
	username string
	password string
}

// NewHTTPRebooter returns a HTTPRebooter with the provided fields.
func NewHTTPRebooter(c *http.Client, baseURL, username, password string) *HTTPRebooter {
	return &HTTPRebooter{
		client:   c,
		baseURL:  baseURL + rebootEndpoint,
		username: username,
		password: password,
	}
}

// one reboots a single machine by send an HTTP request to the Reboot API
// and returns an error if the response code is not 200 or there is a timeout.
func (r *HTTPRebooter) one(toReboot node.Node) error {
	rebootURL := r.baseURL + "?host=" + toReboot.Name

	// Create the HTTP request
	request, err := newHTTPRequest(http.MethodPost, rebootURL, nil)
	if err != nil {
		log.WithError(err).Error("Cannot create HTTP request.")
		return err
	}

	// Add HTTP authentication if needed.
	if r.username != "" && r.password != "" {
		request.SetBasicAuth(r.username, r.password)
	}

	// Send the reboot request and check for errors.
	response, err := clientDo(r, request)
	if err != nil {
		log.WithError(err).Error("Cannot send reboot request.")
		metricRebootRequests.WithLabelValues(toReboot.Name, toReboot.Site, "reboot", "failure").Add(1)
		return err
	}
	defer response.Body.Close()

	body, err := readAll(response.Body)
	if err != nil {
		log.WithError(err).Error(err)
		metricRebootRequests.WithLabelValues(toReboot.Name, toReboot.Site, "reboot", "failure").Add(1)
		return err
	}

	if response.StatusCode != http.StatusOK {
		log.Error(string(body))
		metricRebootRequests.WithLabelValues(toReboot.Name, toReboot.Site, "reboot", "failure").Add(1)
		return errors.New(string(body))
	}

	metricRebootRequests.WithLabelValues(toReboot.Name, toReboot.Site, "reboot", "success").Add(1)

	log.WithFields(log.Fields{"node": toReboot.Name}).Debug(string(body))
	return nil
}

// Many reboots an array of machines and returns a map of
// machineName -> error for each element for which the rebootMany failed.
func (r *HTTPRebooter) Many(toReboot []node.Node) map[string]error {
	errors := make(map[string]error)

	if len(toReboot) == 0 {
		log.Info("There are no nodes to reboot.")
		return errors
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
		err := r.one(c)
		if err != nil {
			errors[c.Name] = err
		}
	}

	return errors
}
