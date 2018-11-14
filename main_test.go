/*
The rebot tool identifies machines on the M-Lab infrastructure that are not
reachable anymore and should be rebooted (according to various criteria) and
attempts to reboot them through iDRAC.
*/
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
	"github.com/prometheus/common/model"
)

var (
	fakeProm          *PrometheusMockClient
	fakeOfflineSwitch *model.Sample
	fakeOfflineNode   *model.Sample

	testMins = 15

	history = map[string]candidate{
		"test": candidate{
			Name:       "test",
			LastReboot: time.Now(),
		},
	}
)

func init() {
	fakeProm = NewPrometheusMockClient()
	now := model.Time(time.Now().Unix())

	fakeOfflineSwitch = CreateSample(map[string]string{
		"instance": "s1.iad0t.measurement-lab.org",
		"job":      "blackbox-targets",
		"module":   "icmp",
		"site":     "iad0t",
	}, 0, now)

	var offlineSwitches = model.Vector{
		fakeOfflineSwitch,
	}

	fakeOfflineNode = CreateSample(map[string]string{
		"instance": "mlab1.lba01.measurement-lab.org:806",
		"job":      "blackbox-targets",
		"machine":  "mlab1.lba01.measurement-lab.org",
		"module":   "ssh_v4_online",
		"service":  "ssh806",
		"site":     "lba01",
	}, 0, now)

	offlineNodes := model.Vector{
		fakeOfflineNode,
	}

	fakeProm.Register(switchQuery, offlineSwitches)
	fakeProm.Register(fmt.Sprintf(nodeQuery, testMins, testMins, testMins), offlineNodes)
}

func Test_getOfflineSites(t *testing.T) {
	tests := []struct {
		name    string
		want    map[string]*model.Sample
		wantErr bool
	}{
		{
			name: "success",
			want: map[string]*model.Sample{
				"iad0t": fakeOfflineSwitch,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getOfflineSites(fakeProm)
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
		minutes int
		want    model.Vector
		wantErr bool
	}{
		{
			name:    "success",
			minutes: testMins,
			want: model.Vector{
				fakeOfflineNode,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getOfflineNodes(fakeProm, tt.minutes)
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
	err := ioutil.WriteFile("credentials", cred, 0644)
	if err != nil {
		panic(err)
	}
}

func teardownCredentials() {
	err := os.Remove("credentials")
	if err != nil {
		panic(err)
	}
}

func Test_getCredentials(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		want  string
		want1 string
	}{
		{
			name:  "success",
			path:  "credentials",
			want:  "testuser",
			want1: "testpass",
		},
	}
	setupCredentials()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := getCredentials(tt.path)
			if got != tt.want {
				t.Errorf("getCredentials() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("getCredentials() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
	teardownCredentials()
}

func setupCandidateHistory() {
	json, err := json.Marshal(history)
	rtx.Must(err, "Cannot marshal the candidates history!")

	err = ioutil.WriteFile("history", json, 0644)
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
			path: "history",
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
	removeFiles("history", "invalidhistory")
}

func Test_writeCandidateHistory(t *testing.T) {
	tests := []struct {
		name             string
		path             string
		candidateHistory map[string]candidate
	}{
		{
			name:             "success",
			path:             "history",
			candidateHistory: history,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeCandidateHistory(tt.path, tt.candidateHistory)
		})
	}

	removeFiles("history")
}
