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
	"github.com/m-lab/rebot/healthcheck"
	"github.com/m-lab/rebot/history"
	"github.com/m-lab/rebot/node"
	"github.com/m-lab/rebot/promtest"
	"github.com/m-lab/rebot/reboot"
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
)

var (
	config api.Config
	client api.Client
	prom   promtest.PromClient

	historyPath     string
	credentialsPath string
	rebootCmd       string
	oneshot         bool

	fDryRun     bool
	fOneshot    bool
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

// filterRecent filters out nodes that were rebooted less than 24 hours ago.
func filterRecent(candidates []node.Node, candidateHistory map[string]node.History) []node.Node {
	filtered := make([]node.Node, 0)

	for _, candidate := range candidates {
		history, ok := candidateHistory[candidate.Name]
		if ok {
			// This candidate has been down before.
			// Check to see if the previous time was w/in the past 24 hours
			if time.Now().Sub(history.LastReboot) > 24*time.Hour {
				filtered = append(filtered, candidate)
			} else {
				log.WithFields(log.Fields{"machine": history.Name, "LastReboot": history.LastReboot}).Info("The node was rebooted recently - skipping it.")
			}
		} else {
			// New candidate - just add it to the list.
			filtered = append(filtered, candidate)
		}
	}

	return filtered
}

// parseFlags reads the runtime flags.
func parseFlags() {
	flag.Parse()

	if fDryRun {
		log.Info("Dry run, no node will be rebooted and the history file will not be updated.")
	}

	if fOneshot {
		oneshot = true
	}
}

func readEnv() {
	if os.Getenv("REBOT_ONESHOT") == "1" {
		oneshot = true
	}
}

// checkAndReboot implements Rebot's reboot logic.
func checkAndReboot(h map[string]node.History) {
	// Query for offline switches
	sites, err := healthcheck.GetOfflineSites(prom)
	if err != nil {
		log.Error("Unable to retrieve offline sites from Prometheus")
		return
	}

	// Query for offline nodes
	nodes, err := healthcheck.GetOfflineNodes(prom, defaultMins)
	if err != nil {
		log.Error("Unable to retrieve offline nodes from Prometheus")
		return
	}

	offline := healthcheck.FilterOfflineSites(sites, nodes)
	toReboot := filterRecent(offline, h)

	metricRebooted.Reset()

	if !fDryRun {
		reboot.Many(rebootCmd, toReboot)
	}

	for _, n := range toReboot {
		metricRebooted.WithLabelValues(n.Name, n.Site).Set(1)
	}

	history.Upsert(toReboot, h)

}

// cleanup waits for a termination signal, writes the candidates' history
// and exits.
func cleanup(h map[string]node.History) {
	log.Info("Cleaning up...")
	history.Write(historyPath, h)
}

// promMetrics serves Prometheus metrics over HTTP.
func promMetrics() {
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(fListenAddr, nil))
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

// init initializes the Prometheus metrics and drops any passed flags into
// global variables.
func init() {
	historyPath = defaultHistoryPath
	credentialsPath = defaultCredentialsPath
	rebootCmd = defaultRebootCmd

	log.SetLevel(log.DebugLevel)

	flag.BoolVar(&fDryRun, "dryrun", false,
		"Do not reboot anything, just list.")
	flag.BoolVar(&fOneshot, "oneshot", false,
		"Execute just once, do not loop.")
	flag.StringVar(&fListenAddr, "web.listen-address", ":9999",
		"Address to listen on for telemetry.")
	prometheus.MustRegister(metricRebooted)
}

func main() {
	readEnv()
	parseFlags()

	initPrometheusClient()
	go promMetrics()

	// First, check to see if there's an existing candidate history file
	// and make sure we always write it back on exit.
	candidateHistory := history.Read(historyPath)

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cleanup(candidateHistory)
	}()

	for {
		checkAndReboot(candidateHistory)

		if oneshot {
			log.Info("Done.")
			cleanup(candidateHistory)
			os.Exit(0)
		}

		log.Info("Done. Going to sleep for ", defaultIntervalMins, " minutes...")
		time.Sleep(defaultIntervalMins * time.Minute)
	}
}
