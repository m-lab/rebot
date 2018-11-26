package main

import (
	"reflect"
	"testing"

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
		want    []candidate
		wantErr bool
	}{
		{
			name:    "success",
			prom:    fakeProm,
			minutes: testMins,
			want: []candidate{
				candidate{
					Name: "mlab1.iad0t.measurement-lab.org",
					Site: "iad0t",
				},
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

	candidates := []candidate{
		candidate{
			Name: "mlab1.iad0t.measurement-lab.org",
			Site: "iad0t",
		},
	}

	tests := []struct {
		name       string
		sites      map[string]*model.Sample
		candidates []candidate
		want       []candidate
	}{
		{
			name: "success-filtered-node-when-site-offline",
			sites: map[string]*model.Sample{
				"iad0t": fakeOfflineSwitch,
			},
			candidates: candidates,
			want:       []candidate{},
		},
		{
			name: "success-offline-node-returned",
			sites: map[string]*model.Sample{
				"iad1t": fakeOfflineSwitch,
			},
			candidates: candidates,
			want: []candidate{
				candidate{
					Name: "mlab1.iad0t.measurement-lab.org",
					Site: "iad0t",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filterOfflineSites(tt.sites, tt.candidates); !(len(got) == 0 && len(tt.want) == 0) && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterOfflineSites() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_filterRecent(t *testing.T) {

	// Nodes where no previous reboot was present
	noHistory := []candidate{
		candidate{
			Name: "mlab2.iad1t.measurement-lab.org",
			Site: "iad1t",
		},
	}

	// Nodes where LastReboot is before 24hrs ago.
	rebootable := []candidate{
		candidate{
			Name: "mlab2.iad0t.measurement-lab.org",
			Site: "iad0t",
		},
	}

	// Nodes where LastReboot is within the last 24hrs.
	notRebootable := []candidate{
		candidate{
			Name: "mlab1.iad0t.measurement-lab.org",
			Site: "iad0t",
		},
		candidate{
			Name: "mlab1.iad1t.measurement-lab.org",
			Site: "iad1t",
		},
	}
	tests := []struct {
		name             string
		candidates       []candidate
		candidateHistory map[string]candidate
		want             []candidate
	}{
		{
			name:             "success-no-history",
			candidates:       noHistory,
			candidateHistory: history,
			want: []candidate{
				candidate{
					Name: "mlab2.iad1t.measurement-lab.org",
					Site: "iad1t",
				},
			},
		},
		{
			name:             "success-rebootable",
			candidates:       rebootable,
			candidateHistory: history,
			want: []candidate{
				candidate{
					Name: "mlab2.iad0t.measurement-lab.org",
					Site: "iad0t",
				},
			},
		},
		{
			name:             "success-not-rebootable",
			candidates:       notRebootable,
			candidateHistory: history,
			want:             []candidate{},
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
