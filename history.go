package main

import (
	"encoding/json"
	"io/ioutil"
	"time"

	"github.com/m-lab/go/rtx"
	log "github.com/sirupsen/logrus"
)

// Struct to hold history of a given service's outages
type candidate struct {
	Name       string
	LastReboot time.Time
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
