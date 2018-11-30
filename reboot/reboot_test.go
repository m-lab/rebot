package reboot

import (
	"reflect"
	"testing"

	"github.com/m-lab/rebot/node"
)

const testRebootCmd = "./drac_test.sh"

func Test_rebootOne(t *testing.T) {
	tests := []struct {
		name     string
		toReboot node.Node
		wantErr  bool
	}{
		{
			name: "success-exit-status-zero",
			toReboot: node.Node{
				Name: "mlab1.lga0t.measurement-lab.org",
				Site: "lga0t",
			},
		},
		{
			name: "failure-exit-status-not-zero",
			toReboot: node.Node{
				Name: "mlab4.lga0t.measurement-lab.org",
				Site: "lga0t",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := one(testRebootCmd, tt.toReboot); (err != nil) != tt.wantErr {
				t.Errorf("rebootOne() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_rebootMany(t *testing.T) {
	toReboot := []node.Node{
		node.Node{
			Name: "mlab1.lga0t.measurement-lab.org",
			Site: "lga0t",
		},
		node.Node{
			Name: "mlab2.lga0t.measurement-lab.org",
			Site: "lga0t",
		},
	}
	want := map[string]error{}

	t.Run("success-all-machines-rebooted", func(t *testing.T) {
		if got := Many(testRebootCmd, toReboot); !reflect.DeepEqual(got, want) {
			t.Errorf("rebootMany() = %v, want %v", got, want)
		}
	})

	// mlab4.* machines always returns a non-zero exit code in drac_test.sh.
	toReboot = []node.Node{
		node.Node{
			Name: "mlab4.lga0t.measurement-lab.org",
			Site: "lga0t",
		},
	}

	t.Run("failure-exit-code-non-zero", func(t *testing.T) {
		got := Many(testRebootCmd, toReboot)
		if err, ok := got["mlab4.lga0t.measurement-lab.org"]; !ok || err == nil {
			t.Errorf("rebootMany() = %v, key not in map or err == nil", got)
		}
	})

	t.Run("success-empty-slice", func(t *testing.T) {
		got := Many(testRebootCmd, []node.Node{})
		if got == nil || len(got) != 0 {
			t.Errorf("rebootMany() = %v, error map not empty.", got)
		}
	})

	toReboot = []node.Node{
		node.Node{
			Name: "mlab1.lga0t.measurement-lab.org",
			Site: "lga0t",
		},
		node.Node{
			Name: "mlab2.lga0t.measurement-lab.org",
			Site: "lga0t",
		},
		node.Node{
			Name: "mlab3.lga0t.measurement-lab.org",
			Site: "lga0t",
		},
		node.Node{
			Name: "mlab1.lga1t.measurement-lab.org",
			Site: "lga1t",
		},
		node.Node{
			Name: "mlab2.lga1t.measurement-lab.org",
			Site: "lga1t",
		},
		node.Node{
			Name: "mlab3.lga1t.measurement-lab.org",
			Site: "lga1t",
		},
	}
	t.Run("success-too-many-nodes", func(t *testing.T) {
		got := Many(testRebootCmd, toReboot)
		if got == nil || len(got) != 0 {
			t.Errorf("rebootMany() = %v, error map not empty.", got)
		}
	})

}
