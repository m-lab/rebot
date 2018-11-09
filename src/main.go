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

	"github.com/prometheus/common/model"

	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/client_golang/api/prometheus/v1"
)

/// Struct to hold history of a given service's outages
type candidate struct {
	Name       string
	LastReboot time.Time
}

type basicAuthRoundTripper struct {
	username string
	password string
	rt       http.RoundTripper
}

func NewBasicAuthRoundTripper(username, password string, rt http.RoundTripper) http.RoundTripper {
	return &basicAuthRoundTripper{username, password, rt}
}

func (rt *basicAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(req.Header.Get("Authorization")) != 0 {
		return rt.rt.RoundTrip(req)
	}
	//req = utilnet.CloneRequest(req)
	req.SetBasicAuth(rt.username, rt.password)
	return rt.rt.RoundTrip(req)
}

func (rt *basicAuthRoundTripper) WrappedRoundTripper() http.RoundTripper { return rt.rt }

// These correspond to the headers used in pkg/apis/authentication.  We don't want the package dependency,
// but you must not change the values.
const (
	// ImpersonateUserHeader is used to impersonate a particular user during an API server request
	ImpersonateUserHeader = "Impersonate-User"

	// ImpersonateGroupHeader is used to impersonate a particular group during an API server request.
	// It can be repeated multiplied times for multiple groups.
	ImpersonateGroupHeader = "Impersonate-Group"

	// ImpersonateUserExtraHeaderPrefix is a prefix for a header used to impersonate an entry in the
	// extra map[string][]string for user.Info.  The key for the `extra` map is suffix.
	// The same key can be repeated multiple times to have multiple elements in the slice under a single key.
	// For instance:
	// Impersonate-Extra-Foo: one
	// Impersonate-Extra-Foo: two
	// results in extra["Foo"] = []string{"one", "two"}
	ImpersonateUserExtraHeaderPrefix = "Impersonate-Extra-"
)

func getStats(username string, password string) (model.Vector, error) {
	/// Takes two strings, representing the username and
	/// password for the Prometheus API, and runs an
	/// HTTP request against mlab-oti.
	/// The query being used is:
	/// (
	/// sum_over_time(probe_success{service="ssh806", module="ssh_v4_online"}[15m]) < 15
	/// )
	/// and on(machine)
	/// (
	/// gmx_machine_maintenance == 0
	/// )

	const QUERY = `(sum_over_time(probe_success{service="ssh806", module="ssh_v4_online"}[15m]) < 15)
				    and on (machine)
				   (gmx_machine_maintenance == 0)`
	config := api.Config{
		Address:      "https://prometheus.mlab-oti.measurementlab.net",
		RoundTripper: NewBasicAuthRoundTripper(username, password, http.DefaultTransport),
	}
	client, _ := api.NewClient(config)

	ClientAPI := v1.NewAPI(client)

	values, err := ClientAPI.Query(context.Background(), QUERY, time.Now())
	if err != nil {
		return nil, err
	}

	return values.(model.Vector), err
}

func getCredentials() (string, string) {
	/// Reads the Prometheus API credentials from the /tmp/credentials
	/// file. It expects a two line file, with username on the first line
	/// and password on the second. Returns a tuple of strings with the
	/// first item being the username and second the password.

	/// TODO (ross) Figure out how to get credentials into the file
	/// Best option is probably Travis secrets.
	file, err := os.Open("/tmp/credentials")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	username, err := reader.ReadBytes('\n')
	if err != nil {
		log.Fatal()
	}
	password, err := reader.ReadBytes('\n')
	if err != nil {
		log.Fatal()
	}

	return string(bytes.Trim(username, "\n")), string(bytes.Trim(password, "\n"))
}

func main() {
	// Call prometheus API for ssh806 service over 15m
	// Sum should be 15. If < 15 query again to see if up now

	// First, check to see if there's an existing candidate history file
	var candidateHistory map[string]candidate
	file, err := ioutil.ReadFile("/tmp/candidateHistory.json")
	if err != nil {
		// There is no existing candidate history file...
		candidateHistory = make(map[string]candidate)
	} else {
		json.Unmarshal(file, &candidateHistory)
	}

	user, pass := getCredentials()
	siteStats, _ := getStats(user, pass)
	var candidates []string
	for _, value := range siteStats {
		if value.Value != 15 {
			candidates = append(candidates, string(value.Metric["machine"]))
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
	if err != nil {
		log.Fatal(err)
	}
	err = ioutil.WriteFile("/tmp/candidateHistory.json", newCandidates, 0644)
}