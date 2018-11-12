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