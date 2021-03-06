package history

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/m-lab/go/rtx"
	"github.com/m-lab/rebot/node"
	log "github.com/sirupsen/logrus"
)

const (
	testHistoryPath = "history"
)

var (
	fakeHist = map[string]node.History{
		"mlab1.iad0t.measurement-lab.org": node.NewHistory(
			"mlab1.iad0t.measurement-lab.org", "iad0t", time.Now()),
		"mlab2.iad0t.measurement-lab.org": node.NewHistory(
			"mlab2.iad0t.measurement-lab.org", "iad0t",
			time.Now().Add(-25*time.Hour)),
		"mlab1.iad1t.measurement-lab.org": node.NewHistory(
			"mlab1.iad1t.measurement-lab.org", "iad1t",
			time.Now().Add(-23*time.Hour)),
	}
)

func setupCandidateHistory() {
	json, err := json.Marshal(fakeHist)
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
			log.Fatalln(err)
		}
	}
}

func Test_readCandidateHistory(t *testing.T) {
	tests := []struct {
		name string
		path string
		want map[string]node.History
	}{
		{
			name: "success",
			path: testHistoryPath,
			want: fakeHist,
		},
		{
			name: "file not existing",
			path: "notfound",
			want: map[string]node.History{},
		},
		{
			name: "invalid history",
			path: "invalidhistory",
			want: map[string]node.History{},
		},
	}

	setupCandidateHistory()
	defer removeFiles(testHistoryPath, "invalidhistory")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Read(tt.path)

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
		candidateHistory map[string]node.History
	}{
		{
			name:             "success",
			path:             testHistoryPath,
			candidateHistory: fakeHist,
		},
	}
	defer removeFiles(testHistoryPath)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Write(tt.path, tt.candidateHistory)
		})
	}
}

func cloneHistory(h map[string]node.History) map[string]node.History {
	newHistory := map[string]node.History{}
	for k, v := range h {
		newHistory[k] = v
	}

	return newHistory
}
func Test_updateHistory(t *testing.T) {
	nodes := []node.Node{
		node.New("mlab1.iad0t.measurement-lab.org", "iad0t"),
		node.New("mlab1.iad1t.measurement-lab.org", "iad1t"),
	}

	testHistory := cloneHistory(fakeHist)

	t.Run("success", func(t *testing.T) {
		Update(nodes, testHistory)

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

	testHistory = cloneHistory(fakeHist)
	t.Run("success-empty-nodes-slice", func(t *testing.T) {
		Update([]node.Node{}, testHistory)

		if !cmp.Equal(testHistory, fakeHist) {
			t.Errorf("updateHistory() = %v, want %v", testHistory, fakeHist)
		}

	})

	n := []node.Node{
		node.New("mlab2.iad1t.measurement-lab.org", "iad1t"),
	}

	t.Run("success-new-candidate", func(t *testing.T) {
		Update(n, testHistory)

		// Check that the new candidate was added and time is within the last
		// minute.
		v, ok := testHistory["mlab2.iad1t.measurement-lab.org"]
		if !ok {
			t.Errorf("updateHistory() = %v, want %v", testHistory, fakeHist)
		}

		if !v.LastReboot.After(time.Now().Add(-1 * time.Minute)) {
			t.Errorf("updateHistory() did not update LastReboot for node %v.", v.Name)
		}
	})
}

func TestUpdateStatus(t *testing.T) {
	nodes := []node.Node{
		node.New("mlab1.iad0t.measurement-lab.org", "iad0t"),
		node.New("mlab1.iad1t.measurement-lab.org", "iad1t"),
	}

	testHistory := cloneHistory(fakeHist)

	UpdateStatus(nodes, testHistory)

	for _, v := range testHistory {
		if v.Status == node.NotObserved {
			t.Errorf("UpdateStatus() did not update Status for node %v.", v.Name)
		}
	}

}

func TestUpdateStatusObservedOffline(t *testing.T) {
	nodes := []node.Node{
		node.New("mlab1.iad0t.measurement-lab.org", "iad0t"),
		node.New("mlab1.iad1t.measurement-lab.org", "iad1t"),
	}

	testHistory := cloneHistory(fakeHist)

	UpdateStatus(nodes, testHistory)

	for _, v := range nodes {
		el, ok := testHistory[v.Name]
		if !ok || el.Status != node.ObservedOffline {
			t.Errorf("UpdateStatus() did not update Status for node %v.", v.Name)
		}
	}
}

func TestUpdateStatusObservedOnline(t *testing.T) {
	testHistory := cloneHistory(fakeHist)

	empty := []node.Node{}

	UpdateStatus(empty, testHistory)

	for _, v := range testHistory {

		el, _ := testHistory[v.Name]
		if el.Status != node.ObservedOnline {
			t.Errorf("UpdateStatus() did not update Status for node %v (Status: %v).", el.Name, el.Status)
		}
	}

}
