package main

import (
	"os/exec"

	log "github.com/sirupsen/logrus"
)

// rebootOne reboots a single machine by calling the reboot command
// and returns an error if the exit status is not zero.
func rebootOne(toReboot candidate) error {
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

// rebootMany reboots an array of machines and returns a map of
// machineName -> error for each element for which the rebootMany failed.
func rebootMany(toReboot []candidate) map[string]error {
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
		log.WithFields(log.Fields{"node": c.Name, "dryrun": fDryRun}).Info("Rebooting node...")
		if !fDryRun {
			err := rebootOne(c)
			if err != nil {
				errors[c.Name] = err
			}
		}

		metricRebooted.WithLabelValues(c.Name, c.Site).Set(1)

	}

	return errors
}
