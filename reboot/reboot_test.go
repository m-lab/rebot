package reboot

import (
	"reflect"
	"testing"

	"github.com/m-lab/go/prometheusx/promtest"
	"github.com/m-lab/rebot/node"
)

func Test_rebootMany(t *testing.T) {

	toReboot := []node.Node{
		{
			Name: "mlab1.lga0t.measurement-lab.org",
			Site: "lga0t",
		},
		{
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
		{
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
		{
			Name: "mlab1.lga0t.measurement-lab.org",
			Site: "lga0t",
		},
		{
			Name: "mlab2.lga0t.measurement-lab.org",
			Site: "lga0t",
		},
		{
			Name: "mlab3.lga0t.measurement-lab.org",
			Site: "lga0t",
		},
		{
			Name: "mlab1.lga1t.measurement-lab.org",
			Site: "lga1t",
		},
		{
			Name: "mlab2.lga1t.measurement-lab.org",
			Site: "lga1t",
		},
		{
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

func TestMetrics(t *testing.T) {
	metricDRACOps.WithLabelValues("x", "x", "x", "x")
	promtest.LintMetrics(t)
}
