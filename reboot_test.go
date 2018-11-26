package main

import (
	"reflect"
	"testing"
)

func Test_rebootOne(t *testing.T) {
	rebootCmd = testRebootCmd
	tests := []struct {
		name     string
		toReboot candidate
		wantErr  bool
	}{
		{
			name: "success-exit-status-zero",
			toReboot: candidate{
				Name: "mlab1.lga0t.measurement-lab.org",
				Site: "lga0t",
			},
		},
		{
			name: "failure-exit-status-not-zero",
			toReboot: candidate{
				Name: "mlab4.lga0t.measurement-lab.org",
				Site: "lga0t",
			},
			wantErr: true,
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

	toReboot := []candidate{
		candidate{
			Name: "mlab1.lga0t.measurement-lab.org",
			Site: "lga0t",
		},
		candidate{
			Name: "mlab2.lga0t.measurement-lab.org",
			Site: "lga0t",
		},
	}
	want := map[string]error{}

	t.Run("success-all-machines-rebooted", func(t *testing.T) {
		if got := rebootMany(toReboot); !reflect.DeepEqual(got, want) {
			t.Errorf("rebootMany() = %v, want %v", got, want)
		}
	})

	// mlab4.* machines always returns a non-zero exit code in drac_test.sh.
	toReboot = []candidate{
		candidate{
			Name: "mlab4.lga0t.measurement-lab.org",
			Site: "lga0t",
		},
	}

	t.Run("failure-exit-code-non-zero", func(t *testing.T) {
		got := rebootMany(toReboot)
		if err, ok := got["mlab4.lga0t.measurement-lab.org"]; !ok || err == nil {
			t.Errorf("rebootMany() = %v, key not in map or err == nil", got)
		}
	})

}
