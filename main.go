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
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/m-lab/go/rtx"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

const (
	defaultIntervalMins    = 30
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

	fDryRun     bool
	fListenAddr string

	// Prometheus metric for exposing number of rebooted machines on last run.
	metricRebooted = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rebot_last_rebooted",
			Help: "Whether a machine was rebooted during the last run.",
		},
		[]string{
			"machine",
			"site",
		},
	)
)

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

	if fDryRun {
		log.Info("Dry run, no node will be rebooted and the history file will not be updated.")
	}
}

// checkAndReboot implements Rebot's reboot logic.
func checkAndReboot(history map[string]candidate) {
	// Query for offline switches
	sites, err := getOfflineSites(prom)
	rtx.Must(err, "Unable to retrieve offline sites from Prometheus")

	// Query for offline nodes
	nodes, err := getOfflineNodes(prom, defaultMins)
	rtx.Must(err, "Unable to retrieve offline nodes from Prometheus")

	offline := filterOfflineSites(sites, nodes)
	toReboot := filterRecent(offline, history)

	metricRebooted.Reset()

	toReboot = []candidate{
		candidate{
			Name: "mlab1.lga0t.measurement-lab.org",
			Site: "lga0t",
		},
	}

	rebootMany(toReboot)
	updateHistory(toReboot, history)

}

// cleanup waits for a termination signal, writes the candidates' history
// and exits.
func cleanup(c chan os.Signal, history map[string]candidate) {
	<-c
	log.Info("Cleaning up...")
	writeCandidateHistory(historyPath, history)
	os.Exit(0)
}

// promMetrics serves Prometheus metrics over HTTP.
func promMetrics() {
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(fListenAddr, nil))
}

// init initializes the Prometheus metrics and drops any passed flags into
// global variables.
func init() {
	historyPath = defaultHistoryPath
	credentialsPath = defaultCredentialsPath
	rebootCmd = defaultRebootCmd

	log.SetLevel(log.DebugLevel)

	flag.BoolVar(&fDryRun, "dryrun", false,
		"Do not reboot anything, just list.")
	flag.StringVar(&fListenAddr, "web.listen-address", ":9999",
		"Address to listen on for telemetry.")
	prometheus.MustRegister(metricRebooted)
}

func main() {
	parseFlags()
	initPrometheusClient()
	go promMetrics()

	// First, check to see if there's an existing candidate history file
	// and make sure we always write it back on exit.
	candidateHistory := readCandidateHistory(historyPath)

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go cleanup(c, candidateHistory)

	for {
		checkAndReboot(candidateHistory)

		log.Info("Done. Going to sleep for ", defaultIntervalMins, " minutes...")
		time.Sleep(defaultIntervalMins * time.Minute)
	}
}
