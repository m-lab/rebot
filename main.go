/*
The rebot tool identifies machines on the M-Lab infrastructure that are not
reachable anymore and should be rebooted (according to various criteria) and
attempts to reboot them through iDRAC.
*/
package main

import (
	"context"
	"flag"
	"math/rand"
	"net/http"
	"time"

	"github.com/m-lab/go/memoryless"
	"github.com/m-lab/go/prometheusx"

	"github.com/m-lab/go/flagx"
	"github.com/m-lab/go/rtx"
	"github.com/m-lab/rebot/healthcheck"
	"github.com/m-lab/rebot/history"
	"github.com/m-lab/rebot/node"
	"github.com/m-lab/rebot/promtest"
	"github.com/m-lab/rebot/reboot"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
)

const (
	defaultMins        = 15
	defaultHistoryPath = "/tmp/candidateHistory.json"
	defaultProject     = "mlab-sandbox"

	// Default timeout for reboot requests. This is intentionally long to
	// accommodate for nodes that are slow to respond and should be higher
	// than the Reboot API's BMC connection timeout.
	clientTimeout = 90 * time.Second
)

var (
	prom promtest.PromClient

	historyPath    string
	rebootAddr     string
	rebootUsername string
	rebootPassword string
	promUsername   string
	promPassword   string

	dryRun  bool
	oneshot bool

	listenAddr string
	project    string

	minSleepTime time.Duration
	maxSleepTime time.Duration
	sleepTime    time.Duration

	// Prometheus metric for last time a machine was rebooted.
	metricLastRebootTs = promauto.NewGaugeVec(
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
	metricTotalReboots = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "rebot_reboot_total",
			Help: "Total number of reboots since startup.",
		},
	)

	metricOffline = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "rebot_machines_offline",
			Help: "Number of machines currently offline. It excludes " +
				"machines that Rebot is ignoring because the switch at the " +
				"site is down.",
		},
	)

	ctx, cancel = context.WithCancel(context.Background())

	newRebooter = func(client *http.Client, baseURL, username,
		password string) Rebooter {
		return reboot.NewHTTPRebooter(client, baseURL, username, password)
	}
)

// Rebooter is an interface that allows to test reboot.HTTPRebooter.
type Rebooter interface {
	Many([]node.Node) map[string]error
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
func checkAndReboot(h map[string]node.History, rebooter *reboot.HTTPRebooter) {
	offline, err := healthcheck.GetOfflineNodes(prom, defaultMins)

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
		rebooter.Many(toReboot)
	}

	for _, n := range toReboot {
		metricLastRebootTs.WithLabelValues(n.Name, n.Site).SetToCurrentTime()
	}

	metricTotalReboots.Add(float64(len(toReboot)))

	history.Update(toReboot, h)
	history.Write(defaultHistoryPath, h)

}

// initPrometheusClient initializes a Prometheus client with HTTP basic
// authentication. If we are running main() in a test, prom will be set
// already, thus we won't replace it.
func initPrometheusClient() {
	if prom == nil {
		config := api.Config{
			Address: "https://" + promUsername + ":" + promPassword +
				"@prometheus." + project + ".measurementlab.net",
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

	log.SetLevel(log.DebugLevel)

	flag.BoolVar(&dryRun, "dryrun", false,
		"Do not reboot anything, just list.")
	flag.BoolVar(&oneshot, "oneshot", false,
		"Execute just once, do not loop.")
	flag.StringVar(&listenAddr, "listenaddr", ":9999",
		"Address to listen on for telemetry.")
	flag.StringVar(&rebootAddr, "reboot.addr", "",
		"Reboot API instance to send reboot request to.")
	flag.StringVar(&rebootUsername, "reboot.username", "",
		"Username for the Reboot API.")
	flag.StringVar(&rebootPassword, "reboot.password", "",
		"Password for the Reboot API.")
	flag.StringVar(&promUsername, "prometheus.username", "",
		"Username for Prometheus.")
	flag.StringVar(&promPassword, "prometheus.password", "",
		"Password for Prometheus.")
	flag.StringVar(&project, "project", defaultProject,
		"Project to use for Prometheus.")
	flag.DurationVar(&sleepTime, "sleeptime", 30*time.Minute,
		"How long to sleep between reboot attempts on average")
	// TODO: decide if min and max really need to be so close to avg. Rule of thumb
	// in the memoryless docs suggests that 3 Minutes and 75 minutes might be
	// better choices.
	flag.DurationVar(&minSleepTime, "minsleeptime", 15*time.Minute,
		"Minimum time to sleep between reboot attempts")
	flag.DurationVar(&maxSleepTime, "maxsleeptime", 45*time.Minute,
		"Maximum time to sleep between reboot attempts")
}

func main() {
	flag.Parse()
	rtx.Must(flagx.ArgsFromEnv(flag.CommandLine), "Could not parse env vars")

	initPrometheusClient()
	srv := prometheusx.MustServeMetrics()
	defer srv.Shutdown(ctx)

	// First, check to see if there's an existing candidate history file.
	candidateHistory := history.Read(historyPath)

	// Create the HTTP client to send requests to the API.
	client := &http.Client{
		Timeout: clientTimeout,
	}

	// Create the HTTPRebooter.
	rebooter := reboot.NewHTTPRebooter(client, rebootAddr, rebootUsername, rebootPassword)

	defer cancel()

	rand.Seed(time.Now().UTC().UnixNano())

	memoryless.Run(
		ctx,
		func() { checkAndReboot(candidateHistory, rebooter) },
		memoryless.Config{Min: minSleepTime, Expected: sleepTime, Max: maxSleepTime, Once: oneshot})
}
