package main

import (
	"reflect"
	"testing"
)

func Test_rebootOne(t *testing.T) {
	rebootCmd = testRebootCmd
	tests := []struct {
		name     string
		toReboot string
		wantErr  bool
	}{
		{
			name:     "success-exit-status-zero",
			toReboot: "mlab1.lga0t.measurement-lab.org",
		},
		{
			name:     "failure-exit-status-not-zero",
			toReboot: "mlab4.lga0t.measurement-lab.org",
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := rebootOne(tt.toReboot); (err != nil) != tt.wantErr {
				t.Errorf("rebootOne() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_rebootMany(t *testing.T) {
	rebootCmd = testRebootCmd

	toReboot := []string{
		"mlab1.lga0t.measurement-lab.org",
		"mlab2.lga0t.measurement-lab.org",
	}
	want := map[string]error{}

	t.Run("success-all-machines-rebooted", func(t *testing.T) {
		if got := rebootMany(toReboot); !reflect.DeepEqual(got, want) {
			t.Errorf("rebootMany() = %v, want %v", got, want)
		}
	})

	// mlab4.* machines always returns a non-zero exit code in drac_test.sh.
	toReboot = []string{
		"mlab4.lga0t.measurement-lab.org",
	}

	t.Run("failure-exit-code-non-zero", func(t *testing.T) {
		got := rebootMany(toReboot)
		if err, ok := got["mlab4.lga0t.measurement-lab.org"]; !ok || err == nil {
			t.Errorf("rebootMany() = %v, key not in map or err == nil", got)
		}
	})

}
