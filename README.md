ReBot
======

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