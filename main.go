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
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"time"

	"github.com/m-lab/go/rtx"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	log "github.com/sirupsen/logrus"
)

// Prometheus HTTP client's interface
type promClient interface {
	Query(context.Context, string, time.Time) (model.Value, error)
}

// Struct to hold history of a given service's outages
type candidate struct {
	Name       string
	LastReboot time.Time
}

const (
	defaultMins            = 15
	defaultCredentialsPath = "/tmp/credentials"
	defaultHistoryPath     = "/tmp/candidateHistory.json"
	defaultRebootCmd       = "drac.py"
	nodeQuery              = `(label_replace(sum_over_time(probe_success{service="ssh806", module="ssh_v4_online"}[%[1]dm]) == 0,
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

var (
	config api.Config
	client api.Client
	prom   promClient

	historyPath     string
	credentialsPath string
	rebootCmd       string

	fDryRun = flag.Bool("dryrun", false, "Do not reboot anything, just list.")
)

func init() {
	historyPath = defaultHistoryPath
	credentialsPath = defaultCredentialsPath
	rebootCmd = defaultRebootCmd

	log.SetLevel(log.DebugLevel)
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

// getCredentials reads the Prometheus API credentials from the
// provided path.
// It expects a two line file, with username on the first line and password
// on the second. Returns a tuple of strings with the first item being the
// username and second the password.
//
// TODO(roberto): get these from env.
func getCredentials(path string) (string, string) {
	file, err := os.Open(path)
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
func readCandidateHistory(path string) map[string]candidate {
	var candidateHistory map[string]candidate
	file, err := ioutil.ReadFile(path)

	if err != nil {
		// There is no existing candidate history file -> return empty map.
		return make(map[string]candidate)
	}

	err = json.Unmarshal(file, &candidateHistory)

	if err != nil {
		log.Warn("Cannot unmarshal the candidates' history file - ignoring it. ", err)
		return make(map[string]candidate)
	}

	return candidateHistory
}

// writeCandidateHistory serializes a string -> candidate map to a JSON file.
// If the map cannot be serialized or the file cannot be written, it exits.
func writeCandidateHistory(path string, candidateHistory map[string]candidate) {
	newCandidates, err := json.Marshal(candidateHistory)
	rtx.Must(err, "Cannot marshal the candidates history!")

	err = ioutil.WriteFile(path, newCandidates, 0644)
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

// updateHistory updates the LastReboot field for all the candidates named in
// the nodes slice. If a candidate did not previously exist, it creates a
// new one.
func updateHistory(nodes []string, history map[string]candidate) {
	if len(nodes) == 0 {
		return
	}

	log.WithFields(log.Fields{"nodes": nodes}).Info("Updating history...")
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

// initPrometheusClient initializes a Prometheus client with HTTP basic
// authentication. If we are running main() in a test, prom will be set
// already, thus we won't replace it.
func initPrometheusClient() {
	if prom == nil {
		user, pass := getCredentials(credentialsPath)

		config = api.Config{
			Address: "https://" + user + ":" + pass + "@prometheus.mlab-oti.measurementlab.net",
		}

		client, err := api.NewClient(config)
		rtx.Must(err, "Unable to initialize a new client!")

		prom = v1.NewAPI(client)
	}
}

// rebootOne reboots a single machine by calling the reboot command
// and returns an error if the exit status is not zero.
func rebootOne(toReboot string) error {
	cmd := exec.Command(rebootCmd, "reboot", toReboot)
	output, err := cmd.Output()

	if err != nil {
		log.Error(err)
		return err
	}

	log.Debug(string(output))
	log.WithFields(log.Fields{"node": toReboot}).Info("Reboot command successfully sent.")
	return nil
}

// rebootMany reboots an array of machines and returns a map of
// machineName -> error for each element for which the rebootMany failed.
func rebootMany(toReboot []string) map[string]error {
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

	for _, m := range toReboot {
		log.WithFields(log.Fields{"node": m}).Info("Rebooting node...")
		err := rebootOne(m)
		if err != nil {
			errors[m] = err
		}
	}

	return errors
}

// parseFlags reads the runtime flags.
func parseFlags() {
	flag.Parse()

	if *fDryRun {
		log.Info("Dry run, no node will be rebooted and the history file will not be updated.")
	}
}

func main() {
	parseFlags()
	initPrometheusClient()

	// First, check to see if there's an existing candidate history file
	candidateHistory := readCandidateHistory(historyPath)

	// Query for offline switches
	sites, err := getOfflineSites(prom)
	rtx.Must(err, "Unable to retrieve offline sites from Prometheus")

	// Query for offline nodes
	nodes, err := getOfflineNodes(prom, defaultMins)
	rtx.Must(err, "Unable to retrieve offline nodes from Prometheus")

	offline := filterOfflineSites(sites, nodes)
	toReboot := filterRecent(offline, candidateHistory)

	if !*fDryRun {
		rebootMany(toReboot)
		updateHistory(toReboot, candidateHistory)
		writeCandidateHistory(historyPath, candidateHistory)
	}

	log.Info("Done.")
}
