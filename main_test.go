package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/m-lab/go/rtx"
	"github.com/m-lab/rebot/promtest"
	"github.com/prometheus/common/model"
)

const (
	testCredentialsPath = "credentials"
	testHistoryPath     = "history"
)

var (
	fakeProm          *promtest.PrometheusMockClient
	fakePromErr       *promtest.PrometheusMockClient
	fakeOfflineSwitch *model.Sample
	fakeOfflineNode   *model.Sample

	offlineNodes model.Vector

	testMins = 15

	history = map[string]candidate{
		"mlab1.iad0t.measurement-lab.org": candidate{
			Name:       "mlab1.iad0t.measurement-lab.org",
			LastReboot: time.Now(),
		},
		"mlab2.iad0t.measurement-lab.org": candidate{
			Name:       "mlab2.iad0t.measurement-lab.org",
			LastReboot: time.Now().Add(-25 * time.Hour),
		},
		"mlab1.iad1t.measurement-lab.org": candidate{
			Name:       "mlab1.iad1t.measurement-lab.org",
			LastReboot: time.Now().Add(-23 * time.Hour),
		},
	}
)

func init() {
	fakeProm = promtest.NewPrometheusMockClient()
	// This client does not have any registered query, thus it always
	// returns an error.
	fakePromErr = promtest.NewPrometheusMockClient()

	now := model.Time(time.Now().Unix())

	fakeOfflineSwitch = promtest.CreateSample(map[string]string{
		"instance": "s1.iad0t.measurement-lab.org",
		"job":      "blackbox-targets",
		"module":   "icmp",
		"site":     "iad0t",
	}, 0, now)

	var offlineSwitches = model.Vector{
		fakeOfflineSwitch,
	}

	fakeOfflineNode = promtest.CreateSample(map[string]string{
		"instance": "mlab1.iad0t.measurement-lab.org:806",
		"job":      "blackbox-targets",
		"machine":  "mlab1.iad0t.measurement-lab.org",
		"module":   "ssh_v4_online",
		"service":  "ssh806",
		"site":     "iad0t",
	}, 0, now)

	offlineNodes = model.Vector{
		fakeOfflineNode,
	}

	fakeProm.Register(switchQuery, offlineSwitches, nil)
	fakeProm.Register(fmt.Sprintf(nodeQuery, testMins, testMins, testMins), offlineNodes, nil)
}

func Test_getOfflineSites(t *testing.T) {
	tests := []struct {
		name    string
		prom    promClient
		want    map[string]*model.Sample
		wantErr bool
	}{
		{
			name: "success",
			want: map[string]*model.Sample{
				"iad0t": fakeOfflineSwitch,
			},
			prom: fakeProm,
		},
		{
			name:    "error",
			prom:    fakePromErr,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getOfflineSites(tt.prom)
			if (err != nil) != tt.wantErr {
				t.Errorf("getOfflineSites() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getOfflineSites() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getOfflineNodes(t *testing.T) {
	tests := []struct {
		name    string
		prom    promClient
		minutes int
		want    model.Vector
		wantErr bool
	}{
		{
			name:    "success",
			prom:    fakeProm,
			minutes: testMins,
			want: model.Vector{
				fakeOfflineNode,
			},
		},
		{
			name:    "error",
			prom:    fakePromErr,
			minutes: testMins,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getOfflineNodes(tt.prom, tt.minutes)
			if (err != nil) != tt.wantErr {
				t.Errorf("getOfflineNodes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getOfflineNodes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func setupCredentials() {
	cred := []byte("testuser\ntestpass\n")
	err := ioutil.WriteFile(testCredentialsPath, cred, 0644)
	if err != nil {
		panic(err)
	}
}

func teardownCredentials() {
	err := os.Remove(testCredentialsPath)
	if err != nil {
		panic(err)
	}
}

func Test_getCredentials(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantUser string
		wantPass string
	}{
		{
			name:     "success",
			path:     testCredentialsPath,
			wantUser: "testuser",
			wantPass: "testpass",
		},
	}
	setupCredentials()
	defer teardownCredentials()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := getCredentials(tt.path)
			if got != tt.wantUser {
				t.Errorf("getCredentials() got = %v, want %v", got, tt.wantUser)
			}
			if got1 != tt.wantPass {
				t.Errorf("getCredentials() got1 = %v, want %v", got1, tt.wantPass)
			}
		})
	}
}

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

func Test_filterOfflineSites(t *testing.T) {

	tests := []struct {
		name  string
		sites map[string]*model.Sample
		nodes model.Vector
		want  []string
	}{
		{
			name: "success-filtered-node-when-site-offline",
			sites: map[string]*model.Sample{
				"iad0t": fakeOfflineSwitch,
			},
			nodes: offlineNodes,
			want:  []string{},
		},
		{
			name: "success-offline-node-returned",
			sites: map[string]*model.Sample{
				"iad0t": fakeOfflineSwitch,
			},
			nodes: model.Vector{
				promtest.CreateSample(map[string]string{
					"instance": "mlab1.iad1t.measurement-lab.org:806",
					"job":      "blackbox-targets",
					"machine":  "mlab1.iad1t.measurement-lab.org",
					"module":   "ssh_v4_online",
					"service":  "ssh806",
					"site":     "iad1t",
				}, 0, model.Time(time.Now().Unix())),
			},
			want: []string{"mlab1.iad1t.measurement-lab.org"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filterOfflineSites(tt.sites, tt.nodes); !(len(got) == 0 && len(tt.want) == 0) && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterOfflineSites() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_filterRecent(t *testing.T) {

	// Nodes where no previous reboot was present
	noHistory := []string{
		"mlab2.iad1t.measurement-lab.org",
	}

	// Nodes where LastReboot is before 24hrs ago.
	rebootable := []string{
		"mlab2.iad0t.measurement-lab.org",
	}

	// Nodes where LastReboot is within the last 24hrs.
	notRebootable := []string{
		"mlab1.iad0t.measurement-lab.org",
		"mlab1.iad1t.measurement-lab.org",
	}
	tests := []struct {
		name             string
		nodes            []string
		candidateHistory map[string]candidate
		want             []string
	}{
		{
			name:             "success-no-history",
			nodes:            noHistory,
			candidateHistory: history,
			want: []string{
				"mlab2.iad1t.measurement-lab.org",
			},
		},
		{
			name:             "success-rebootable",
			nodes:            rebootable,
			candidateHistory: history,
			want: []string{
				"mlab2.iad0t.measurement-lab.org",
			},
		},
		{
			name:             "success-not-rebootable",
			nodes:            notRebootable,
			candidateHistory: history,
			want:             []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filterRecent(tt.nodes, tt.candidateHistory); !(len(got) == 0 && len(tt.want) == 0) && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterRecent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_main(t *testing.T) {
	setupCandidateHistory()
	setupCredentials()
	defer removeFiles(testHistoryPath, testCredentialsPath, "invalidhistory")

	prom = fakeProm
	historyPath = testHistoryPath
	credentialsPath = testCredentialsPath
	fmt.Println(switchQuery)
	t.Run("success", func(t *testing.T) {
		main()
	})

}

func Test_initPrometheusClient(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "success",
		},
	}

	setupCredentials()
	defer removeFiles(testCredentialsPath)

	credentialsPath = testCredentialsPath
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initPrometheusClient()
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

	tests := []struct {
		name    string
		nodes   []string
		history map[string]candidate
	}{
		{
			name:    "success",
			nodes:   nodes,
			history: cloneHistory(history),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateHistory(tt.nodes, tt.history)

			// Check that LastReboot is within the last minute for nodes
			// in the nodes slice.
			for _, node := range nodes {
				candidate, ok := tt.history[node]
				if !ok {
					t.Errorf("%v missing in the history map.", node)
				}

				if !candidate.LastReboot.After(time.Now().Add(-1 * time.Minute)) {
					t.Errorf("updateHistory() did not update LastReboot for node %v.", candidate.Name)
				}

			}
		})
	}
}
