[![GoDoc](https://godoc.org/github.com/m-lab/rebot?status.svg)](https://godoc.org/github.com/m-lab/rebot) [![Build Status](https://travis-ci.org/m-lab/rebot.svg?branch=master)](https://travis-ci.org/m-lab/rebot) [![Coverage Status](https://coveralls.io/repos/github/m-lab/rebot/badge.svg?branch=master)](https://coveralls.io/github/m-lab/rebot?branch=master) [![Go Report Card](https://goreportcard.com/badge/github.com/m-lab/rebot)](https://goreportcard.com/report/github.com/m-lab/rebot)

ReBot
======
The rebot tool identifies machines on the M-Lab infrastructure that are not
reachable anymore and should be rebooted (according to various criteria) and
attempts to reboot them through iDRAC.

Criteria for reboot candidates
---

This is the list of criteria ReBot will check to determine if a machine needs
to be rebooted.

- machine is offline - port 806 down for the last 15m
- machine is not lame-ducked - lame_duck_node is not 1
- site and machine are not in GMX maintenance - gmx_machine_maintenance and gmx_site_maintenance are not 1
- switch is online - probe_success{instance=~"s1.*", module="icmp"} has been 0 for the last 15m
- there are no NDT tests running - rate(inotify_extension_create_total{ext=".s2c_snaplog"}[15m]) is 0 or not present
- metrics are actually being collected for all probes (i.e. prometheus was up)
  - count_over_time(probe_success{service="ssh806", module="ssh_v4_online"}[15m]) >= 14

Additionally, ReBot checks the following:
- the machine has not been rebooted already in the last 24hrs
- no more than 5 machines should be rebooted together at any time
