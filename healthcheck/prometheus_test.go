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

	fakeProm.Register(fmt.Sprintf(CandidatesQuery, testMins), offlineNodes, nil)
}

func Test_GetOfflineNodes(t *testing.T) {
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
				node.New("mlab1.iad0t.measurement-lab.org", "iad0t"),
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
				t.Errorf("GetOfflineNodes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetOfflineNodes() = %v, want %v", got, tt.want)
			}
		})
	}
}
