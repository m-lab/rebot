package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/m-lab/go/rtx"
)

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
		want map[string]candidate
	}{
		{
			name: "success",
			path: testHistoryPath,
			want: history,
		},
		{
			name: "file not existing",
			path: "notfound",
			want: map[string]candidate{},
		},
		{
			name: "invalid history",
			path: "invalidhistory",
			want: map[string]candidate{},
		},
	}

	setupCandidateHistory()
	defer removeFiles(testHistoryPath, "invalidhistory")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := readCandidateHistory(tt.path)

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
		candidateHistory map[string]candidate
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
			writeCandidateHistory(tt.path, tt.candidateHistory)
		})
	}
}

func cloneHistory(h map[string]candidate) map[string]candidate {
	newHistory := map[string]candidate{}
	for k, v := range h {
		newHistory[k] = v
	}

	return newHistory
}
func Test_updateHistory(t *testing.T) {
	nodes := []string{
		"mlab1.iad0t.measurement-lab.org",
		"mlab1.iad1t.measurement-lab.org",
	}

	testHistory := cloneHistory(history)

	t.Run("success", func(t *testing.T) {
		updateHistory(nodes, testHistory)

		// Check that LastReboot is within the last minute for nodes
		// in the nodes slice.
		for _, node := range nodes {
			candidate, ok := testHistory[node]
			if !ok {
				t.Errorf("%v missing in the history map.", node)
			}

			if !candidate.LastReboot.After(time.Now().Add(-1 * time.Minute)) {
				t.Errorf("updateHistory() did not update LastReboot for node %v.", candidate.Name)
			}

		}
	})
}
