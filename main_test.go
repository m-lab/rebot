/*
The rebot tool identifies machines on the M-Lab infrastructure that are not
reachable anymore and should be rebooted (according to various criteria) and
attempts to reboot them through iDRAC.
*/
package main

import (
	"reflect"
	"testing"
	"time"

	"github.com/prometheus/common/model"
)

var (
	fakeProm          *PrometheusMockClient
	fakeOfflineSwitch *model.Sample
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

	fakeProm.Register(switchQuery, offlineSwitches)
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
