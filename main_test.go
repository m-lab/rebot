package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/m-lab/go/prometheusx"

	"github.com/m-lab/go/osx"
	"github.com/m-lab/rebot/healthcheck"
	"github.com/m-lab/rebot/node"
	"github.com/m-lab/rebot/promtest"
	"github.com/prometheus/common/model"
	log "github.com/sirupsen/logrus"
)

const (
	testMins = 15
)

var (
	fakeProm *promtest.PrometheusMockClient
)

func init() {
	now := model.Time(time.Now().Unix())
	fakeProm = promtest.NewPrometheusMockClient()
	fakeOfflineSwitch := promtest.CreateSample(map[string]string{
		"instance": "s1.iad0t.measurement-lab.org",
		"job":      "blackbox-targets",
		"module":   "icmp",
		"site":     "iad0t",
	}, 0, now)

	offlineSwitches := model.Vector{
		fakeOfflineSwitch,
	}

	fakeOfflineNode := promtest.CreateSample(map[string]string{
		"instance": "mlab1.iad0t.measurement-lab.org:806",
		"job":      "blackbox-targets",
		"machine":  "mlab1.iad0t.measurement-lab.org",
		"module":   "ssh_v4_online",
		"service":  "ssh806",
		"site":     "iad0t",
	}, 0, now)

	offlineNodes := model.Vector{
		fakeOfflineNode,
	}

	fakeProm.Register(healthcheck.SwitchQuery, offlineSwitches, nil)
	fakeProm.Register(fmt.Sprintf(healthcheck.NodeQuery, testMins), offlineNodes, nil)

	prom = fakeProm
}

const (
	testCredentialsPath = "credentials"
	testHistoryPath     = "history"
	testRebootCmd       = "./drac_test.sh"
)

func setupCredentials() {
	cred := []byte("testuser\ntestpass\n")
	err := ioutil.WriteFile(testCredentialsPath, cred, 0644)
	if err != nil {
		log.Fatalln(err)
	}
}

func removeFiles(files ...string) {
	for _, file := range files {
		err := os.Remove(file)
		if err != nil {
			log.Fatalln(err)
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
		node.New("mlab2.iad1t.measurement-lab.org", "iad1t"),
	}

	// Nodes where LastReboot is before 24hrs ago.
	rebootable := []node.Node{
		node.New("mlab2.iad0t.measurement-lab.org", "iad0t"),
	}

	// Nodes where LastReboot is within the last 24hrs.
	notRebootable := []node.Node{
		node.New("mlab1.iad0t.measurement-lab.org", "iad0t"),
		node.New("mlab1.iad1t.measurement-lab.org", "iad1t"),
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

func Test_main_oneshot(t *testing.T) {
	restore := osx.MustSetenv("ONESHOT", "1")
	defer restore()

	ctx, cancel = context.WithCancel(context.Background())
	listenAddr = ":9000"

	main()

	cancel()
	time.Sleep(2 * time.Second)

}

func Test_main_multi(t *testing.T) {
	restore := osx.MustSetenv("ONESHOT", "0")
	defer restore()

	ctx, cancel = context.WithCancel(context.Background())
	listenAddr = ":9001"

	go func() {
		time.Sleep(1 * time.Second)
		cancel()
	}()

	main()
}

func TestMetrics(t *testing.T) {
	prometheusx.LintMetrics(t)
}
