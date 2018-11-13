/*
The rebot tool identifies machines on the M-Lab infrastructure that are not
reachable anymore and should be rebooted (according to various criteria) and
attempts to reboot them through iDRAC.
*/
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/m-lab/go/rtx"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

/// Struct to hold history of a given service's outages
type candidate struct {
	Name       string
	LastReboot time.Time
}

type basicAuthRoundTripper struct {
	username string
	password string
	http.RoundTripper
}

func newBasicAuthRoundTripper(username, password string, rt http.RoundTripper) http.RoundTripper {
	return &basicAuthRoundTripper{username, password, rt}
}

func (rt *basicAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(req.Header.Get("Authorization")) != 0 {
		return rt.RoundTripper.RoundTrip(req)
	}

	req.SetBasicAuth(rt.username, rt.password)
	return rt.RoundTripper.RoundTrip(req)
}

func (rt *basicAuthRoundTripper) WrappedRoundTripper() http.RoundTripper { return rt.RoundTripper }

const (
	defaultMins     = 15
	credentialsPath = "/tmp/credentials"
	historyPath     = "/tmp/candidateHistory.json"
	nodeQuery       = `label_replace(sum_over_time(probe_success{service="ssh806", module="ssh_v4_online"}[%dm]) == 0,
				"site", "$1", "machine", ".+?\\.(.+?)\\..+")
				unless on(machine) gmx_machine_maintenance == 1
				unless on(site) gmx_site_maintenance == 1
				unless on (machine) lame_duck_node == 1
				unless on (machine) count_over_time(probe_success{service="ssh806", module="ssh_v4_online"}[%dm]) < 14
				unless on (machine) rate(inotify_extension_create_total{ext=".s2c_snaplog"}[%dm]) > 0`

	// To determine if a switch is offline, pings are generally more reliable
	// than SNMP scraping.
	switchQuery = `sum_over_time(probe_success{instance=~"s1.*", module="icmp"}[15m]) == 0`
)

var (
	config api.Config
	client api.Client
	prom   v1.API
)

func init() {
	user, pass := getCredentials()

	config = api.Config{
		Address:      "https://prometheus.mlab-oti.measurementlab.net",
		RoundTripper: newBasicAuthRoundTripper(user, pass, http.DefaultTransport),
	}

	client, err := api.NewClient(config)
	rtx.Must(err, "Unable to initialize a new client!")

	prom = v1.NewAPI(client)
}

// getOfflineSites checks for offline sites (switches) in the last N minutes.
// It returns a sitename -> Sample map.
func getOfflineSites() (map[string]*model.Sample, error) {
	offline := make(map[string]*model.Sample)

	values, err := prom.Query(context.Background(), switchQuery, time.Now())
	if err != nil {
		return nil, err
	}

	for _, s := range values.(model.Vector) {
		offline[string(s.Metric["site"])] = s
	}

	return offline, err
}

// getOfflineNodes checks for offline nodes in the last N minutes.
// It returns a Vector of samples.
func getOfflineNodes(minutes int) (model.Vector, error) {

	values, err := prom.Query(context.Background(), fmt.Sprintf(nodeQuery, minutes, minutes, minutes), time.Now())
	if err != nil {
		return nil, err
	}

	return values.(model.Vector), err
}

// getCredentials reads the Prometheus API credentials from the
// /tmp/credentials file.
// It expects a two line file, with username on the first line and password
// on the second. Returns a tuple of strings with the first item being the
// username and second the password.
//
// TODO(roberto): get these from env.
func getCredentials() (string, string) {
	file, err := os.Open(credentialsPath)
	rtx.Must(err, "Cannot open credentials' file")
	defer file.Close()

	reader := bufio.NewReader(file)
	username, err := reader.ReadBytes('\n')
	rtx.Must(err, "Cannot read username from "+credentialsPath)

	password, err := reader.ReadBytes('\n')
	rtx.Must(err, "Cannot read password from"+credentialsPath)

	return string(bytes.Trim(username, "\n")), string(bytes.Trim(password, "\n"))
}

// readCandidateHistory reads a JSON file containing a map of
// string -> candidate. If the file cannot be read or deserialized, it returns
// an empty map.
func readCandidateHistory() map[string]candidate {
	var candidateHistory map[string]candidate
	file, err := ioutil.ReadFile(historyPath)

	if err != nil {
		// There is no existing candidate history file -> return empty map.
		return make(map[string]candidate)
	}

	err = json.Unmarshal(file, &candidateHistory)

	if err != nil {
		log.Println("Cannot unmarshal the candidates' history file - ignoring it")
		log.Println(err)
		return make(map[string]candidate)
	}

	return candidateHistory
}

// writeCandidateHistory serializes a string -> candidate map to a JSON file.
// If the map cannot be serialized or the file cannot be written, it exits.
func writeCandidateHistory(candidateHistory map[string]candidate) {
	newCandidates, err := json.Marshal(candidateHistory)
	rtx.Must(err, "Cannot marshal the candidates history!")

	err = ioutil.WriteFile(historyPath, newCandidates, 0644)
	rtx.Must(err, "Cannot write the candidates history's JSON file!")
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
			log.Println("Ignoring " + machine + " as the switch is offline.")
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
				log.Println("Skipping " + node + " as it was recently rebooted.")
			}
		} else {
			// New candidate - just add it to the list.
			rebootList = append(rebootList, node)
		}
	}

	return rebootList
}

// updateHistory updates the LastReboot field for all the candidates named in
// the nodes slice. If a candidate did not previously exist, it creates a
// new one.
func updateHistory(nodes []string, history map[string]candidate) {
	for _, node := range nodes {
		el, ok := history[node]

		if ok {
			el.LastReboot = time.Now()
			history[node] = el
		} else {
			history[node] = candidate{
				Name:       node,
				LastReboot: time.Now(),
			}
		}
	}
}

func main() {
	// First, check to see if there's an existing candidate history file

	candidateHistory := readCandidateHistory()

	// Query for offline switches
	sites, err := getOfflineSites()
	rtx.Must(err, "Unable to retrieve offline switches from Prometheus")

	// Query for offline nodes
	nodes, err := getOfflineNodes(defaultMins)
	rtx.Must(err, "Unable to retrieve offline nodes from Prometheus")

	offline := filterOfflineSites(sites, nodes)
	toReboot := filterRecent(offline, candidateHistory)

	// TODO(roberto): actually try to reboot the nodes.

	updateHistory(toReboot, candidateHistory)
	writeCandidateHistory(candidateHistory)

}
