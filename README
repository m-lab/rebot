ReBot is a tool for automatically rebooting downed servers.

To generate a list of servers in need of a reboot, please close this repo and on your local machine, execute

./mlab-ssh-outage.sh

That will create an output directory called ssh_outage_<timestamp>. Any node shortnames in a file called reboot-me are in a hard down state for ssh and sshalt, and have not been acknowledged, thus need to be rebooted. If not such file exists, no machines need to be rebooted at the time the script is run.
