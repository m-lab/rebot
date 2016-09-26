#!/bin/bash
#
# Reboot crashed hosts. A host is rebooted if:
# 1) baseList.pl shows ssh (later, host) and sshalt alert as down for the host
# 2) the switch for the site is up
# 3) the host isn't a straggler from the last run
# 4) The host hasn't been rebooted in the last 24 hours
# 5) There are no more than 5 hosts in the current reboot queue
# Any machines that came back from the previous run are recorded. 
#
# TODO: This script sets the nagios server as eb.measurementlab.net. This should
# be changed to import a general definition of the nagios server from a config
# file common to all maintenance scripts. (import mlabconfig ?)
# TODO: Fix baseList bug.
# On eb, this script runs as root because of some kind of bug. If run as a
# regular user, baseList gives an error trying to read the nagios status file,
# even though the file is readable.
# TODO: For cron, this script would have to be run as root, or use Nagios
# authentication because of the baseList.pl permissions, see above TODO.
# TODO: This version generates a trustworthy list of host to reboot right now.
# Next step is to log them and make sure that current host haven't been
# rebooted in the last 24 hours, and that no more than 5 host total are in the
# process of being rebooted.
# TODO: What should we do with any hosts over the 5-host limit? Ignore and wait
# for the next run? Put them at the top of the queue if they are still down
# during the next run?

TIMESTAMP="$(date -u +%F_%H-%M)"
EPOCH_NOW="$(date -u +%s)"
EPOCH_YESTERDAY="$(date -u +%s --date='24 hours ago')"
#REBOT_LOG_DIR="/tmp/rebot"
REBOT_LOG_DIR="/tmp/rebot-testing"
SSH_OUTAGE_TEMP_DIR="${REBOT_LOG_DIR}/ssh_outage"
REBOOT_HISTORY_DIR="${REBOT_LOG_DIR}/reboot_history"
DOWN_NODES_SSHALT="${SSH_OUTAGE_TEMP_DIR}/down_hosts_sshalt"
DOWN_NODES_SSH="${SSH_OUTAGE_TEMP_DIR}/down_hosts_ssh"
DOWN_SWITCHES="${SSH_OUTAGE_TEMP_DIR}/down_switches_ssh"
REBOOT_CANDIDATES="${SSH_OUTAGE_TEMP_DIR}/reboot_candidates"
REBOOT_CANDIDATES_1="${SSH_OUTAGE_TEMP_DIR}/reboot_candidates.1"
REBOOT_ME="${SSH_OUTAGE_TEMP_DIR}/reboot_me"
REBOOT_ATTEMPTED="${REBOOT_HISTORY_DIR}/reboot_attempted"
REBOOT_LOG="${REBOOT_HISTORY_DIR}/reboot_log"
REBOOT_RUNNING_LOG="${REBOOT_HISTORY_DIR}/reboot_running_log"
PROBLEMATIC="${REBOOT_HISTORY_DIR}/problematic"

########################################
# Function: find_all_hosts 
# Authenticate to Nagios to view baseList.pl output for ssh and sshalt
# Ask baseList for show_state=1 (yes, show state),
# service_name=sshalt 
# OR, when fixed, host_name=all
# plugin_output=0 (no, don't show output, not needed)
# show_problem_acknowledged=1 (yes, show whether acknowledged)
# Makes a fresh $SSH_OUTAGE_TEMP_DIR on each run
# Creates files $SSH_OUTAGE_TEMP_DIR/all_hosts_ssh and $SSH_OUTAGE_TEMP_DIR/all_hosts_sshalt
########################################
find_all_hosts() {
echo "###########################"
echo "Starting find_all_hosts()"
rm -r "${SSH_OUTAGE_TEMP_DIR}"
mkdir -p "${SSH_OUTAGE_TEMP_DIR}"
echo "This directory gets recreated with fresh status files every time \
rebot runs." > "${SSH_OUTAGE_TEMP_DIR}/README"
echo "Getting status of ssh and sshalt from Nagios"
for plugin in ssh sshalt; do

# Configure a user's login to Nagios.  For more information on netrc files, see
# the manpage for curl.  If a netrc file doesn't exist, then the user will be
# prompted to enter credentials.
if [ -f ~/.netrc ] && grep -q 'nagios.measurementlab.net' ~/.netrc; then
  nagios_auth='--netrc'
else
  read -p "Nagios login: " nagios_login
  read -s -p "Nagios password: " nagios_pass
  nagios_auth="--user ${nagios_login}:${nagios_pass}"
  echo -e '\n'
fi

curl -s "${nagios_auth}" -o "${SSH_OUTAGE_TEMP_DIR}"/all_hosts_"${plugin}" --digest --netrc \
  "http://nagios.measurementlab.net/baseList?show_state=1&service_name="${plugin}"&plugin_output=0&show_problem_acknowledged=1"
done
}

########################################
# Function: find_down_hosts
# Search the ssh and sshalt reports for hosts in state 2 (bad state),
# duration state 1 ("hard" state, meaning it's been that way for a while),
# and problem_acknowledged state 0 (not acknowledged).
# If there are hosts in state "2 1 0",
# creates files down_hosts_ssh and/or down_hosts_sshalt, and down_switches 
# Find hosts in both the down ssh and sshalt lists
########################################
find_down_hosts() {
echo "###########################"
echo "Starting find_down_hosts()"
echo "Searching baseList output for hosts in hard state 1"
for plugin in "ssh" "sshalt"; do
  # Make the down_hosts file creation idempotent so we can append inside the
  # loop without getting unintended appended node lists from run to run
  if [ -f "${SSH_OUTAGE_TEMP_DIR}/down_hosts_${plugin}" ]; then
    rm "${SSH_OUTAGE_TEMP_DIR}/down_hosts_${plugin}"
  fi
  while read line; do
    host="$(echo $line | awk '{print $1 }')"
    state="$(echo $line | awk '{ print $2 }')"
    hard="$(echo $line | awk '{ print $3 }')"
    problem_acknowledged="$(echo $line | awk '{ print $4 }')"
    if [[ ${state} == 2 ]] && [[ ${hard} == 1 ]] && \
    [[ "$problem_acknowledged" == 0 ]]; then
      echo "${host}" |grep -v ^s| awk -F. '{ print $1"."$2 }' \
      >> "${SSH_OUTAGE_TEMP_DIR}/down_hosts_${plugin}"
      echo "${host}" |grep ^s | awk -F. '{ print $1"."$2 }' \
      >> "${SSH_OUTAGE_TEMP_DIR}/down_switches_${plugin}"
    fi
  done < "${SSH_OUTAGE_TEMP_DIR}/all_hosts_${plugin}"
done
if [[ -s "${DOWN_NODES_SSH}" ]] && [[ -s "${DOWN_NODES_SSHALT}" ]]; then
  echo "There are down hosts to check; continuing"
  else echo "There are no down hosts; exiting"
  exit
fi
}

########################################
# Function: strip out the hosts with a sitename that matches
# the sitename of any down switch
# Input: DOWN_SWITCHES
# Output: REBOOT_CANDIDATES
########################################
find_reboot_candidates() {
echo "###########################"
echo "Starting find_reboot_candidates()"
# If both ssh and sshalt are both down, they go on the "maybe" list
comm --nocheck-order -12 "${DOWN_NODES_SSH}" "${DOWN_NODES_SSHALT}" > \
"${REBOOT_CANDIDATES}"

echo "Contents of REBOOT_CANDIDATES after comparison of ssh and sshalt:"
cat "${REBOOT_CANDIDATES}"
echo ""
# The sshalt plugin doesn't report switches, but this file is risidual of the
# loop above and isn't needed.
if [ -f "${SSH_OUTAGE_TEMP_DIR}/down_switches_sshalt" ]; then
  rm "${SSH_OUTAGE_TEMP_DIR}/down_switches_sshalt"
fi

echo "Now looking for down switches and removing them from REBOOT_CANDIDATES"
if [[ -s "${DOWN_SWITCHES}" ]] ; then
  echo "Down switches:"
  cat "${DOWN_SWITCHES}"
  for line in `cat "${DOWN_SWITCHES}" | awk -F. '{ print $2 }'`; do
    echo "Stripping $line out of REBOOT_CANDIDATES"
    grep -v $line "${REBOOT_CANDIDATES}" > "${REBOOT_CANDIDATES}".tmp
    mv  "${REBOOT_CANDIDATES}".tmp "${REBOOT_CANDIDATES}"
  done
else
  echo "No down switches"
  echo ""
fi ;
echo "New Contents of REBOOT_CANDIDATES:"
cat "${REBOOT_CANDIDATES}"
}

########################################
# Function: did_they_come_back
# If a host from the previous attempt is still a reboot candidate, pull it out
# of reboot candidates and into a still_down file.
# If all hosts from the previous attempt are clean, scrub it from the
# reboot_attempted file, because it succeeded.
# TODO: Maybe this should check the timestamp to make sure the previous attempt
# isn't stale (more than 15 min or 900 sec long)
# TODO: Make sure REBOOT_ATTEMPTED doesn't accidentally collect old runs.
# Outdated attempts should be moved to problematic or something.
# Input/Output: Reads and scrubs from REBOOT_ATTEMPTED and REBOOT_CANDIDATES
# Output PROBLEMATIC 
########################################
did_they_come_back() {
echo "###########################"
echo "Starting did_they_come_back()"
#echo "Contents of REBOOT_ATTEMPTED:"
#cat ${REBOOT_ATTEMPTED}
#echo ""
#echo "Contents of REBOOT_CANDIDATES:"
#cat ${REBOOT_CANDIDATES}
#echo ""
for line in `cat "${REBOOT_ATTEMPTED}"`; do
  echo "Line is:" $line
  for host in `echo $line | awk -F: '{ print $1 }'`; do
    echo "Host is:" $host
    if grep -q $host "${REBOOT_CANDIDATES}" ; then
      cp "${REBOOT_CANDIDATES}" "${REBOOT_CANDIDATES}".$TIMESTAMP
      echo "$host attempted to boot during the last run but is still down. Storing in Problematic and scrubbing from REBOOT_CANDIDATES."
      echo $host" still not up but requested during last run:" $line >> $PROBLEMATIC
      grep -v $host "${REBOOT_CANDIDATES}" > "${REBOOT_CANDIDATES}".tmp
      mv  "${REBOOT_CANDIDATES}".tmp "${REBOOT_CANDIDATES}"
    else
      echo "Logging $host in REBOOT_RUNNING_LOG and scrubbing from REBOOT_ATTEMPTED because its reboot succeeded."
      cp "${REBOOT_ATTEMPTED}" "${REBOOT_ATTEMPTED}".$TIMESTAMP
      grep -v $host "${REBOOT_ATTEMPTED}" > "${REBOOT_ATTEMPTED}".tmp
      mv  "${REBOOT_ATTEMPTED}".tmp "${REBOOT_ATTEMPTED}"
      echo $line >> "${REBOOT_RUNNING_LOG}"
    fi
  done
done
#echo "New Contents of REBOOT_ATTEMPTED (/tmp/rebot-testing/reboot_history/reboot_attempted):"
#cat ${REBOOT_ATTEMPTED}
#echo ""
}

########################################
# Function: has_it_been_24_hrs() 
# Need a check to make sure the reboot_candidates weren't rebooted in
# the last 24 hours
# Next, take current epoch time, epoch time of previous entry for
# this server, subtract previous from current, make sure it's more than 86400s
# or 24 hours. SECONDS_IN_A_DAY=$(($EPOCH_NOW - $EPOCH_YESTERDAY))
# Input: REBOOT_CANDIDATES
# Output: REBOOT_CANDIDATES
########################################
has_it_been_24_hrs() {
echo "###########################"
echo "Starting has_it_been_24_hrs" 

#echo "Contents of REBOOT_CANDIDATES (/tmp/rebot-testing/ssh_outage/reboot_candidates:"
#cat ${REBOOT_CANDIDATES}
#echo "Contents of REBOOT_RUNNING_LOG (/tmp/rebot-testing/reboot_history/reboot_running_log:"
#cat ${REBOOT_RUNNING_LOG}
echo ""
for host in `cat "${REBOOT_CANDIDATES}"`; do
  if grep -q $host "${REBOOT_RUNNING_LOG}" ; then
    # echo "$host found in REBOOT_RUNNING_LOG. Grabbing previous reboot timestamp."
    PREVIOUS_REBOOT=`grep $host $REBOOT_RUNNING_LOG |awk -F: '{print $3 }'`
    # echo "Previous reboot:" $PREVIOUS_REBOOT
    SECONDS_SINCE_REBOOT=$(($EPOCH_NOW - $PREVIOUS_REBOOT ))
    echo "Seconds since "$host"'s previous reboot (should be more than 86400):" $SECONDS_SINCE_REBOOT
    # echo "$host previous reboot:" $PREVIOUS_REBOOT". Seconds since: "$SECONDS_SINCE_REBOOT". Should be more than 86400."
    if [ "${SECONDS_SINCE_REBOOT}" -gt 86400 ]; then
      echo "More than a day since $host was rebooted. Ok to reboot."
    else
      echo "Less than a day since $host was rebooted. Not ok to reboot."
      grep -v $host "${REBOOT_CANDIDATES}" > "${REBOOT_CANDIDATES}".tmp
      mv  "${REBOOT_CANDIDATES}".tmp "${REBOOT_CANDIDATES}"
    fi
  else
    echo "$host is not present in the REBOOT_RUNNING_LOG. Ok to proceed."
  fi
done

echo "New Contents of REBOOT_CANDIDATES (/tmp/rebot-testing/ssh_outage/reboot_candidates:"
for line in `cat "${REBOOT_CANDIDATES}"`; do
  echo $line":"$TIMESTAMP":"$EPOCH_NOW
done
echo ""
}

########################################
# Function: perform_the_reboot
# At this point, the REBOOT_ME list should be clean
# Need to make sure only 5 candidates get rebooted at a time
# and we can perform the drac command and dump it to the attempted list
# Takes REBOOT_ME as input, write to REBOOT_ATTEMPTED
########################################
perform_the_reboot() {
echo "###########################"
echo "Starting perform_the_reboot"
# head -5 "${REBOOT_CANDIDATES} > "${REBOOT_ME}"
if [[ -s "${REBOOT_ME}" ]] ; then
  echo "Please reboot these:"
  cat "${REBOOT_ME}"
  for line in `cat "${REBOOT_ME}"`; do
    echo $line":"$TIMESTAMP":"$EPOCH_NOW >> "${REBOOT_ATTEMPTED}"
    # echo $host":"$TIMESTAMP":"$EPOCH_NOW >> ${REBOOT_LOG}
  done
else
  echo "No machines to reboot."
fi ;
}

########################################
# Run the functions
########################################
find_all_hosts
find_down_hosts
find_reboot_candidates
did_they_come_back
has_it_been_24_hrs
#perform_the_reboot
