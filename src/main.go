package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

/// Struct to hold history of a given service's outages
type Candidate {
	Name string
	LastDown Time
}

/// Structs that replicate the Prometheus query response
/// JSON that we're getting. 'Value' is an interface
/// because that JSON object is an array of two different
/// types, which cannot be expressed in a type-safe language.
type Run struct {
	Status string
	Data   ResultsShell
}

type ResultsShell struct {
	ResultType string
	Result     []Result
}

type Result struct {
	Metric Stats
	Value  []interface{}
}

type Stats struct {
	Instace string
	Job     string
	Machine string
	Module  string
	Service string
}

func getStats(username string, password string) []byte {
	/// Takes two strings, representing the username and
	/// password for the Prometheus API, and runs an
	/// HTTP request against mlab-oti.
	/// The non-urlencoded query is
	/// "sum_over_time(probe_success{service="ssh806", module="ssh_v4_online"}[15m])"
	client := &http.Client{}
	req, err := http.NewRequest("GET", "https://prometheus.mlab-oti.measurementlab.net/api/v1/query?query=sum_over_time%28probe_success%7Bservice%3D%22ssh806%22%2C%20module%3D%22ssh_v4_online%22%7D%5B15m%5D%29", nil)
	req.SetBasicAuth(username, password)
	resp, err := client.Do(req)
	if err != nil {
		// If we can't access Prometheus, just exit
		log.Fatal(err)
	}
	defer resp.Close()
	bodyText, err := ioutil.ReadAll(resp.Body)
	return bodyText
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
	candidate_history = make(map[string]Candidate)
	user, pass := getCredentials()
	promJson := getStats(user, pass)
	var marshalRun Run
	json.Unmarshal(promJson, &marshalRun)
	var candidates []string
	for _, site := range marshalRun.Data.Result {
		if site.Value[1] != "15" {
			candidates = append(candidates, site.Metric.Machine)
		}
	}
	fmt.Println(candidates)
	for _, site := range candidates {

	}
}
