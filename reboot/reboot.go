package reboot

import (
	"os/exec"

	"github.com/m-lab/rebot/healthcheck"

	log "github.com/sirupsen/logrus"
)

const rebootCmd = "drac.py"

// one reboots a single machine by calling the reboot command
// and returns an error if the exit status is not zero.
func one(rebootCmd string, toReboot healthcheck.Node) error {
	cmd := exec.Command(rebootCmd, "reboot", toReboot.Name)
	output, err := cmd.Output()

	if err != nil {
		log.Error(err)
		return err
	}

	log.Debug(string(output))
	log.WithFields(log.Fields{"node": toReboot}).Info("Reboot command successfully sent.")
	return nil
}

// Many reboots an array of machines and returns a map of
// machineName -> error for each element for which the rebootMany failed.
func Many(rebootCmd string, toReboot []healthcheck.Node) map[string]error {
	errors := make(map[string]error)

	if len(toReboot) == 0 {
		log.Info("There are no nodes to reboot.")
		return errors
	}

	// If there are more than 5 nodes to be rebooted, do nothing.
	// TODO(roberto) find a better way to report this case to the caller.
	if len(toReboot) > 5 {
		log.WithFields(log.Fields{"nodes": toReboot}).Error("There are more than 5 nodes offline, skipping.")
		return errors
	}

	log.WithFields(log.Fields{"nodes": toReboot}).Info("These nodes are going to be rebooted.")

	for _, c := range toReboot {
		log.WithFields(log.Fields{"node": c}).Info("Rebooting node...")
		err := one(rebootCmd, c)
		if err != nil {
			errors[c.Name] = err
		}
	}

	return errors
}
