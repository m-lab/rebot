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

// Reads the Prometheus API credentials from the /tmp/credentials file.
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

func main() {
	// First, check to see if there's an existing candidate history file

	candidateHistory := readCandidateHistory()
	ok := true

	// Query for offline switches
	sites, err := getOfflineSites()
	if err != nil {
		ok = false
		log.Println("Unable to retrieve offline switches from Prometheus")
		log.Println(err)
	}

	nodes, err := getOfflineNodes(15)
	if err != nil {
		ok = false
		log.Println("Unable to retrieve offline nodes from Prometheus")
		log.Println(err)
	}

	if ok {
		var candidates []string

		for _, value := range nodes {
			// Ignore machines in sites where the switch is offline.
			site := string(value.Metric["site"])
			if _, ok := sites[site]; !ok {
				candidates = append(candidates, string(value.Metric["machine"]))
			} else {
				println("Ignoring " + site + " as the switch is offline.")
			}

		}

		fmt.Println(candidates)

		var realCandidates []string
		for _, site := range candidates {
			thisCandidate, ok := candidateHistory[site]
			if ok {
				// This candidate has been down before.
				// Check to see if the previous time was w/in the past 24 hours
				if time.Now().Sub(thisCandidate.LastReboot) > 24*time.Hour {
					// If previous incident was more than 24 hours ago,
					// its still a candidate, so add it to the list
					realCandidates = append(realCandidates, thisCandidate.Name)
					// Update the candidate with the current time and update the map
					thisCandidate.LastReboot = time.Now()
					candidateHistory[site] = thisCandidate
				}
			} else {
				// There's no candidate object in the map for this site
				// so we have to create one and add it.
				candidateHistory[site] = candidate{
					Name:       site,
					LastReboot: time.Now(),
				}
				realCandidates = append(realCandidates, site)
			}
		}
		newCandidates, err := json.Marshal(candidateHistory)
		rtx.Must(err, "Cannot marshal the candidates history!")

		err = ioutil.WriteFile(historyPath, newCandidates, 0644)
		rtx.Must(err, "Cannot write the candidates history's JSON file!")
	} else {
		log.Println("Skipping as we could not retrieve data from Prometheus.")
	}
}
