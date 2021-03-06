package history

import (
	"encoding/json"
	"io/ioutil"
	"time"

	"github.com/m-lab/rebot/node"

	"github.com/m-lab/go/rtx"
	log "github.com/sirupsen/logrus"
)

// Read reads a JSON file containing a map of
// string -> candidate. If the file cannot be read or deserialized, it returns
// an empty map.
func Read(path string) map[string]node.History {
	var candidateHistory map[string]node.History
	file, err := ioutil.ReadFile(path)

	if err != nil {
		// There is no existing candidate history file -> return empty map.
		return make(map[string]node.History)
	}

	err = json.Unmarshal(file, &candidateHistory)

	if err != nil {
		log.Warn("Cannot unmarshal the candidates' history file - ignoring it. ", err)
		return make(map[string]node.History)
	}

	return candidateHistory
}

// Write serializes a string -> candidate map to a JSON file.
// If the map cannot be serialized or the file cannot be written, it exits.
func Write(path string, candidateHistory map[string]node.History) {
	newCandidates, err := json.Marshal(candidateHistory)
	rtx.Must(err, "Cannot marshal the candidates history!")

	err = ioutil.WriteFile(path, newCandidates, 0644)
	rtx.Must(err, "Cannot write the candidates history's JSON file!")
}

// UpdateStatus updates the history according to the list of nodes to be
// rebooted on the current run. If a node was in NotObserved status, it will be
// updated to either ObservedOffline, if it's still in the candidates slice, or
// ObservedOnline if it's not.
func UpdateStatus(candidates []node.Node, history map[string]node.History) {

	for _, c := range candidates {
		hist, ok := history[c.Name]
		if ok && hist.Status == node.NotObserved {
			log.WithField("node", hist.Name).Warn("Reboot failed during the last run.")
			hist.Status = node.ObservedOffline
			history[c.Name] = hist
		}
	}

	// If there is any other "NotObserved" at this point, it is online now.
	for k, v := range history {
		if v.Status == node.NotObserved {
			log.WithField("node", v.Name).Info("The node was rebooted successfully during the last run.")
			v.Status = node.ObservedOnline
			history[k] = v
		}
	}
}

// Update updates the LastReboot field for all the candidates named in
// the nodes slice and sets the Status to NotObserved.
// If a candidate did not previously exist, it creates a new one.
func Update(candidates []node.Node, history map[string]node.History) {
	if len(candidates) == 0 {
		return
	}

	log.WithFields(log.Fields{"nodes": candidates}).Info("Updating history...")
	for _, c := range candidates {
		history[c.Name] = node.NewHistory(c.Name, c.Site, time.Now())
	}

}
