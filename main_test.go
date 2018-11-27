package main

import (
	"io/ioutil"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/m-lab/rebot/node"
)

const (
	testCredentialsPath = "credentials"
	testHistoryPath     = "history"
	testRebootCmd       = "./drac_test.sh"
)

func setupCredentials() {
	cred := []byte("testuser\ntestpass\n")
	err := ioutil.WriteFile(testCredentialsPath, cred, 0644)
	if err != nil {
		panic(err)
	}
}

func removeFiles(files ...string) {
	for _, file := range files {
		err := os.Remove(file)
		if err != nil {
			panic(err)
		}
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
	defer removeFiles(testCredentialsPath)
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

func Test_initPrometheusClient(t *testing.T) {

	setupCredentials()
	defer removeFiles(testCredentialsPath)

	credentialsPath = testCredentialsPath
	t.Run("success", func(t *testing.T) {
		initPrometheusClient()
	})
}

func Test_filterRecent(t *testing.T) {

	h := map[string]node.History{
		"mlab1.iad0t.measurement-lab.org": node.NewHistory(
			"mlab1.iad0t.measurement-lab.org", "iad0t", time.Now()),
		"mlab2.iad0t.measurement-lab.org": node.NewHistory(
			"mlab.iad0t.measurement-lab.org", "iad0t",
			time.Now().Add(-25*time.Hour)),
		"mlab1.iad1t.measurement-lab.org": node.NewHistory(
			"mlab1.iad1t.measurement-lab.org", "iad1t",
			time.Now().Add(-23*time.Hour)),
	}

	// Nodes where no previous reboot was present
	noHistory := []node.Node{
		node.NewNode("mlab2.iad1t.measurement-lab.org", "iad1t"),
	}

	// Nodes where LastReboot is before 24hrs ago.
	rebootable := []node.Node{
		node.NewNode("mlab2.iad0t.measurement-lab.org", "iad0t"),
	}

	// Nodes where LastReboot is within the last 24hrs.
	notRebootable := []node.Node{
		node.NewNode("mlab1.iad0t.measurement-lab.org", "iad0t"),
		node.NewNode("mlab1.iad1t.measurement-lab.org", "iad1t"),
	}
	tests := []struct {
		name             string
		candidates       []node.Node
		candidateHistory map[string]node.History
		want             []node.Node
	}{
		{
			name:             "success-no-history",
			candidates:       noHistory,
			candidateHistory: h,
			want:             noHistory,
		},
		{
			name:             "success-rebootable",
			candidates:       rebootable,
			candidateHistory: h,
			want:             rebootable,
		},
		{
			name:             "success-not-rebootable",
			candidates:       notRebootable,
			candidateHistory: h,
			want:             []node.Node{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filterRecent(tt.candidates, tt.candidateHistory); !(len(got) == 0 && len(tt.want) == 0) && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterRecent() = %v, want %v", got, tt.want)
			}
		})
	}
}
