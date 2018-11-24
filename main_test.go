package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/m-lab/rebot/promtest"
	"github.com/prometheus/common/model"
)

const (
	testCredentialsPath = "credentials"
	testHistoryPath     = "history"
	testRebootCmd       = "./drac_test.sh"
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
	prom = nil
}

func Test_initPrometheusClient(t *testing.T) {

	setupCredentials()
	defer removeFiles(testCredentialsPath)

	credentialsPath = testCredentialsPath
	t.Run("success", func(t *testing.T) {
		initPrometheusClient()
	})
}
