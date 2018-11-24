package main

import (
	"reflect"
	"testing"
	"time"

	"github.com/m-lab/rebot/promtest"
	"github.com/prometheus/common/model"
)

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
