<<<<<<< HEAD
ReBot is a tool for automatically rebooting downed servers.

To generate a list of servers in need of a reboot, please close this repo and on your local machine, execute

./mlab-ssh-outage.sh

That will create an output directory called ssh_outage_<timestamp>. Any node shortnames in a file called reboot-me are in a hard down state for ssh and sshalt, and have not been acknowledged, thus need to be rebooted. If not such file exists, no machines need to be rebooted at the time the script is run.
=======
ReBot
======

Criteria for reboot candidates
---

This is the list of criteria ReBot will check to determine if a machine needs
to be rebooted. 

- machine is offline - port 806 down for the last 15m
- machine is not lame-ducked
- site and machine are not in GMX maintenance
- switch is online (ping / SNMP tests)
- there are no NDT tests running
- metrics are actually being collected for all probes (i.e. prometheus was up)

Additionally, ReBot checks the following:
- the machine has not been rebooted already in the last 24hrs
- no more than 5 machines should be rebooted together at any time
>>>>>>> Added README.md.
