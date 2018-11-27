package history

import (
	"encoding/json"
	"io/ioutil"
	"time"

	"github.com/m-lab/rebot/healthcheck"

	"github.com/m-lab/go/rtx"
	log "github.com/sirupsen/logrus"
)

// NodeHistory holds the history of a machine's outages
type NodeHistory struct {
	Name       string
	Site       string
	LastReboot time.Time
}

// NewNodeHistory returns a new machineHistory.
func NewNodeHistory(name string, site string, lastReboot time.Time) NodeHistory {
	return NodeHistory{
		Name:       name,
		Site:       site,
		LastReboot: lastReboot,
	}
}

// Read reads a JSON file containing a map of
// string -> candidate. If the file cannot be read or deserialized, it returns
// an empty map.
func Read(path string) map[string]NodeHistory {
	var candidateHistory map[string]NodeHistory
	file, err := ioutil.ReadFile(path)

	if err != nil {
		// There is no existing candidate history file -> return empty map.
		return make(map[string]NodeHistory)
	}

	err = json.Unmarshal(file, &candidateHistory)

	if err != nil {
		log.Warn("Cannot unmarshal the candidates' history file - ignoring it. ", err)
		return make(map[string]NodeHistory)
	}

	return candidateHistory
}

// Write serializes a string -> candidate map to a JSON file.
// If the map cannot be serialized or the file cannot be written, it exits.
func Write(path string, candidateHistory map[string]NodeHistory) {
	newCandidates, err := json.Marshal(candidateHistory)
	rtx.Must(err, "Cannot marshal the candidates history!")

	err = ioutil.WriteFile(path, newCandidates, 0644)
	rtx.Must(err, "Cannot write the candidates history's JSON file!")
}

// Upsert updates the LastReboot field for all the candidates named in
// the nodes slice. If a candidate did not previously exist, it creates a
// new one.
func Upsert(candidates []healthcheck.Node, history map[string]NodeHistory) {
	if len(candidates) == 0 {
		return
	}

	log.WithFields(log.Fields{"nodes": candidates}).Info("Updating history...")
	for _, c := range candidates {
		el, ok := history[c.Name]
		if ok {
			el.LastReboot = time.Now()
			history[c.Name] = el
		} else {
			history[c.Name] = NewNodeHistory(c.Name, c.Site, time.Now())
		}
	}

}
