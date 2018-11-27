package history

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/m-lab/rebot/healthcheck"

	"github.com/google/go-cmp/cmp"
	"github.com/m-lab/go/rtx"
)

const (
	testHistoryPath = "history"
)

var ()

func setupCandidateHistory() {
	json, err := json.Marshal(history)
	rtx.Must(err, "Cannot marshal the candidates history!")

	err = ioutil.WriteFile(testHistoryPath, json, 0644)
	rtx.Must(err, "Cannot write the candidates history's JSON file!")

	err = ioutil.WriteFile("invalidhistory", []byte("notjson"), 0644)
	rtx.Must(err, "Cannot write the invalid history's JSON file!")
}

func removeFiles(files ...string) {
	for _, file := range files {
		err := os.Remove(file)
		if err != nil {
			panic(err)
		}
	}
}

func Test_readCandidateHistory(t *testing.T) {
	tests := []struct {
		name string
		path string
		want map[string]MachineHistory
	}{
		{
			name: "success",
			path: testHistoryPath,
			want: history,
		},
		{
			name: "file not existing",
			path: "notfound",
			want: map[string]MachineHistory{},
		},
		{
			name: "invalid history",
			path: "invalidhistory",
			want: map[string]MachineHistory{},
		},
	}

	setupCandidateHistory()
	defer removeFiles(testHistoryPath, "invalidhistory")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReadCandidateHistory(tt.path)

			// Here we use go-cmp as time.Time will not be exactly the same
			// after marshalling/unmarshalling. In particular, the monotonic
			// clock field is not marshalled, and this makes
			// reflect.DeepEqual() not give the expected result.
			if !cmp.Equal(got, tt.want) {
				t.Errorf("readCandidateHistory() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_writeCandidateHistory(t *testing.T) {
	tests := []struct {
		name             string
		path             string
		candidateHistory map[string]MachineHistory
	}{
		{
			name:             "success",
			path:             testHistoryPath,
			candidateHistory: history,
		},
	}
	defer removeFiles(testHistoryPath)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Write(tt.path, tt.candidateHistory)
		})
	}
}

func cloneHistory(h map[string]MachineHistory) map[string]MachineHistory {
	newHistory := map[string]MachineHistory{}
	for k, v := range h {
		newHistory[k] = v
	}

	return newHistory
}
func Test_updateHistory(t *testing.T) {
	nodes := []healthcheck.Node{
		healthcheck.NewNode("mlab1.iad0t.measurement-lab.org", "iad0t"),
		healthcheck.NewNode("mlab1.iad1t.measurement-lab.org", "iad1t"),
	}

	testHistory := cloneHistory(history)

	t.Run("success", func(t *testing.T) {
		Upsert(nodes, testHistory)

		// Check that LastReboot is within the last minute for nodes
		// in the nodes slice.
		for _, candidate := range nodes {
			candidate, ok := testHistory[candidate.Name]
			if !ok {
				t.Errorf("%v missing in the history map.", candidate.Name)
			}

			if !candidate.LastReboot.After(time.Now().Add(-1 * time.Minute)) {
				t.Errorf("updateHistory() did not update LastReboot for node %v.", candidate.Name)
			}

		}
	})
}
