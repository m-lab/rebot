#!/bin/bash

##### For Python:#####
##!/usr/bin/env python
#
## rebot
## Rebot runs on the nagios server b/c it can access baselist.pl without authentication. It is also whitelisted for DRAC commands.
#
#def usage():
#    return """
#    rebot.py -- Discover crashed servers and reboot them. 
#
#        ./fetch.py --script procs --rerun status=255 --list
#        ./fetch.py --script procs --rerun status=255
#        ./fetch.py --command "ps ax" --nodelist list.txt
#"""
#
##First, ask Nagios if there are any reboot candidates.
#def discover():
#    "Discover reboot candidates and decide whether they should be kicked."
#    return

#    If nagios says it's fine, do nothing.

## Here, make a nagios status-finding function. For the moment, just hand-set it to 0 or 1.
#  Call baseList.pl. If nagios status for a server is down for both ssh and sshalt, status=1.
#No reboot needed=0
#Rebootable=1

NAGIOS_STATUS=0
HOSTNAME=mlab1
SITENAME=nuq0t
SHORTNAME=HOSTNAME.SITENAME
MINUTES_SINCE_REBOOT=1440
HOURS_SINCE_REBOOT=MINUTES_SINCE_REBOOT/60
# Should a candidate be rebooted? 
    #If Nagios status is ok, log and exit/sleep.
if [ $NAGIOS_STATUS -eq 0 ]
then 
    echo "All good at $DATE" >> $LOG_FILE 
    # If Nagios status is not ok, move to the next challenge.
else
        # Is the switch accessible via ssh?
            # No? Don't reboot, but do log. Include the decision that prevented the reboot in a “class of event” column.
    if [ ssh s1.$sitename.measurement-lab.org fails ]
    then
        set $SWITCH_OK=no
        echo "Switch s1.$sitename.measurement-lab.org is down. Something may be wrong with the site." >> $LOG_FILE
        # Yes? Save status and move on to the next challenge.
    else
	set $SWITCH_OK=yes
    fi

    # Has it been at least 24 hours since it was rebooted? (Is there a state file for the node and is the reboot timestamp more than 24 hours?)
    if [ $MINUTES_SINCE_REBOOT > 1440 ] 
    then
        # Yes? Move on to the next challenge. 
        set $REBOOT_TIMER_OK=yes
        # No? Don't reboot, but do log. Include the decision that prevented the reboot in a “class of event” column.
    else 
	set $REBOOT_TIME_OK=no
	echo "$SHORTNAME appears down but it was already rebooted $HOURS_SINCE_REBOOT hours ago. Not rebooting." >> $LOG_FILE
    fi

    # Have more than 5 crashed machines been detected during this run?
    if [ $REBOOT_COUNTER=>5 ]
    then
        # Yes? Don't reboot, alert a human because something is wrong.
        set $REBOOT_COUNT_OK=no
        echo "Switch s1.$sitename.measurement-lab.org is down. Something may be wrong with the site." >> $LOG_FILE
    else 
        # No? Reboot and log. Keeping monitoring and report when the server is back up.
    reboot($SHORTNAME)
fi 

#Then take the reboot candidate and do that thing.
##def reboot():
##    "Reboot servers that passed the discovery tests"
##    return

#    Take the short hostname and ask PLC for the DRAC login.    
# This needs persistent auth.
# If this fails b/c the PLC database is down, then boot.planet-lab
drac.py $SHORTNAME
#        # Alternately, dump the DRAC database once a day and draw from that in case 
#PLC is down Rejected because if the PLC can’t be reached for a password, chances are boot.planet-lab.org is also down, therefore nodes can’t boot anyway. So sleep for some amount of time instead
#    Issue a racadm serveraction powercycle
#    Log to a local file and a shared spreadsheet:
#        Timestamp of the machine (Should always be UTC. If the host’s timezone isn’t UTC, that’s a bug.)        Short hostname
#        The challenge responses from above
#        Timestamp when ssh succeeds again from last stanza of challenge subroutine above
#    Write local state file:
#        cd /tmp/rebot/<short node name>
#            { 
#                “reboot”: <timestamp>,
#                “recovery”: <timestamp>,
#            }
#
#

