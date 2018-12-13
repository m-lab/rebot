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
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/m-lab/go/flagx"
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

	dryRun     bool
	oneshot    bool
	listenAddr string

	// Prometheus metric for last time a machine was rebooted.
	metricLastRebootTs = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rebot_last_reboot_timestamp",
			Help: "Timestamp of the last reboot for a machine.",
		},
		[]string{
			"machine",
			"site",
		},
	)

	// Prometheus metric for total number of reboots.
	metricTotalReboots = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "rebot_reboot_total",
			Help: "Total number of reboots since startup.",
		},
	)

	// Prometheus metric for number of DRAC operations executed.
	metricDRACOps = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rebot_drac_operations_total",
			Help: "Total number of DRAC operations run.",
		},
		[]string{
			"machine",
			"site",
			"type",
			"status",
		},
	)

	metricOffline = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "rebot_machines_offline",
			Help: "Number of machines currently offline. It excludes " +
				"machines that Rebot is ignoring because the switch at the " +
				"site is down.",
		},
	)

	ctx, cancel = context.WithCancel(context.Background())
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

// checkAndReboot implements Rebot's reboot logic.
func checkAndReboot(h map[string]node.History) {
	offline, err := healthcheck.GetRebootable(prom, defaultMins)

	metricOffline.Set(float64(len(offline)))

	if !dryRun {
		history.UpdateStatus(offline, h)
	}

	if err != nil {
		log.Error("Unable to retrieve the list of rebootable nodes. " +
			"Is Prometheus reachable?")
		return
	}

	toReboot := filterRecent(offline, h)

	if !dryRun {
		reboot.Many(rebootCmd, toReboot, metricDRACOps)
	}

	for _, n := range toReboot {
		metricLastRebootTs.WithLabelValues(n.Name, n.Site).SetToCurrentTime()
	}

	metricTotalReboots.Add(float64(len(toReboot)))

	history.Update(toReboot, h)
	history.Write(defaultHistoryPath, h)

}

// promMetrics serves Prometheus metrics over HTTP.
func promMetrics() *http.Server {
	srv := &http.Server{Addr: listenAddr}
	handler := http.NewServeMux()
	handler.Handle("/metrics", promhttp.Handler())
	srv.Handler = handler

	go func() {
		err := srv.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	return srv
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

	flag.BoolVar(&dryRun, "dryrun", false,
		"Do not reboot anything, just list.")
	flag.BoolVar(&oneshot, "oneshot", false,
		"Execute just once, do not loop.")
	flag.StringVar(&listenAddr, "listenaddr", ":9999",
		"Address to listen on for telemetry.")
	prometheus.MustRegister(metricLastRebootTs)
	prometheus.MustRegister(metricTotalReboots)
	prometheus.MustRegister(metricDRACOps)
	prometheus.MustRegister(metricOffline)
}

func main() {
	flag.Parse()
	flagx.ArgsFromEnv(flag.CommandLine)

	initPrometheusClient()
	srv := promMetrics()
	defer srv.Shutdown(nil)

	// First, check to see if there's an existing candidate history file.
	candidateHistory := history.Read(historyPath)

	defer cancel()

	rand.Seed(time.Now().UTC().UnixNano())

	for {
		sleepTime := time.Duration(rand.ExpFloat64()*float64(defaultIntervalMins)) * time.Minute
		checkAndReboot(candidateHistory)
		if oneshot {
			break
		}

		log.Info("Done. Going to sleep for ", sleepTime)

		select {
		case <-time.NewTimer(sleepTime).C:
			// continue
		case <-ctx.Done():
			fmt.Println("Returning")
			return
		}
	}
}
