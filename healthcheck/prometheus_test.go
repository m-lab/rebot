package healthcheck

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/m-lab/rebot/node"
	"github.com/m-lab/rebot/promtest"

	"github.com/prometheus/common/model"
)

var (
	fakeProm          *promtest.PrometheusMockClient
	fakePromErr       *promtest.PrometheusMockClient
	fakeOfflineSwitch *model.Sample
	fakeOfflineNode   *model.Sample

	offlineNodes model.Vector

	testMins = 15
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

	fakeProm.Register(SwitchQuery, offlineSwitches, nil)
	fakeProm.Register(fmt.Sprintf(NodeQuery, testMins, testMins, testMins), offlineNodes, nil)
}

func Test_getOfflineSites(t *testing.T) {
	tests := []struct {
		name    string
		prom    promtest.PromClient
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
			got, err := GetOfflineSites(tt.prom)
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
		prom    promtest.PromClient
		minutes int
		want    []node.Node
		wantErr bool
	}{
		{
			name:    "success",
			prom:    fakeProm,
			minutes: testMins,
			want: []node.Node{
				node.NewNode("mlab1.iad0t.measurement-lab.org", "iad0t"),
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
			got, err := GetOfflineNodes(tt.prom, tt.minutes)
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

func Test_filterOfflineSites(t *testing.T) {

	candidates := []node.Node{
		node.NewNode("mlab1.iad0t.measurement-lab.org", "iad0t"),
	}

	tests := []struct {
		name       string
		sites      map[string]*model.Sample
		candidates []node.Node
		want       []node.Node
	}{
		{
			name: "success-filtered-node-when-site-offline",
			sites: map[string]*model.Sample{
				"iad0t": fakeOfflineSwitch,
			},
			candidates: candidates,
			want:       []node.Node{},
		},
		{
			name: "success-offline-node-returned",
			sites: map[string]*model.Sample{
				"iad1t": fakeOfflineSwitch,
			},
			candidates: candidates,
			want: []node.Node{
				node.NewNode("mlab1.iad0t.measurement-lab.org", "iad0t"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FilterOfflineSites(tt.sites, tt.candidates); !(len(got) == 0 && len(tt.want) == 0) && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterOfflineSites() = %v, want %v", got, tt.want)
			}
		})
	}
}
