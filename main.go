/*
The rebot tool identifies machines on the M-Lab infrastructure that are not
reachable anymore and should be rebooted (according to various criteria) and
attempts to reboot them through iDRAC.
*/
package main

import (
	"bufio"
	"bytes"
	"flag"
	"os"

	"github.com/m-lab/go/rtx"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/client_golang/api/prometheus/v1"
	log "github.com/sirupsen/logrus"
)

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
