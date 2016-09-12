#!/bin/bash
#
# Reboot crashed nodes. A node is declared down if baseList.pl shows the ssh and
# sshalt alert as down for the node, and the switch for the site is up.
#
# TODO: This script sets the nagios server as eb.measurementlab.net. This should
# be changed to import a general definition of the nagios server from a config
# file common to all maintenance scripts. (import mlabconfig ?)
# TODO: On eb, this script runs as root because baseList is root executable.
# Should that be changed?
# TODO: For cron, this script would have to be run as root because of the
# baseList.pl permissions, see above TODO.
# TODO: This version generates a trustworthy list of nodes to reboot right now.
# Next step is to log them and make sure that current nodes haven't been
# rebooted in the last 24 hours, and that no more than 5 nodes total are in the
# process of being rebooted.

#TIMESTAMP="$(date +%F_%H-%M)"
#SSH_OUTAGE=ssh_outage_${TIMESTAMP}
REBOT_LOG_DIR="/tmp/rebot"
SSH_OUTAGE="${REBOT_LOG_DIR}/ssh_outage"
DOWN_NODES_SSHALT="${SSH_OUTAGE}/down_nodes_sshalt"
DOWN_NODES_SSH="${SSH_OUTAGE}/down_nodes_ssh"
DOWN_SWITCHES="${SSH_OUTAGE}/down_switches_ssh"
REBOOT_CANDIDATES="${SSH_OUTAGE}/reboot_candidates"
REBOOT_ME="${SSH_OUTAGE}/reboot_me"
REBOOT_LOG="${REBOT_LOG_DIR}/reboot_log"
EPOCH_NOW="$(date -u +%s)"
EPOCH_YESTERDAY="$(date -u +%s --date='24 hours ago')"
########################################
# Function: find_all_nodes 
# Authenticate to Nagios to view baseList.pl output for ssh and sshalt
# Makes a fresh $SSHE_OUTAGE dir on each run
# Creates files $SSH_OUTAGE/all_nodes_ssh and $SSH_OUTAGE/all_nodes_sshalt
########################################
find_all_nodes() {

rm -r "${SSH_OUTAGE}"
mkdir -p "${SSH_OUTAGE}"

# If the script is running on eb, run baseList locally.
# Anywhere else, use the URL of the cgi script on the Nagios server.
echo "Getting status of ssh and sshalt from Nagios"
for plugin in ssh sshalt; do
  if [[ "$(hostname -f)" = "eb.measurementlab.net" ]] ; then
    sudo /etc/nagios3/baseList.pl show_state=1 service_name="${plugin}" \
plugin_output=0&show_problem_acknowledged=1 >> \
"${SSH_OUTAGE}/all_nodes_${plugin}"
  else

# Nested functions aren't really a thing in bash, but save this auth function
# for the rewrite in Python.
# Configure a user's login to Nagios.  For more information on netrc files, see
# the manpage for curl.  If a netrc file doesn't exist, then the user will be
# prompted to enter credentials.
  #nagios_auth() {
    if [ -f ~/.netrc ] && grep -q 'nagios.measurementlab.net' ~/.netrc; then
      nagios_auth='--netrc'
      else
      read -p "Nagios login: " nagios_login
      read -s -p "Nagios password: " nagios_pass
      nagios_auth="--user ${nagios_login}:${nagios_pass}"
      echo -e '\n'
    fi
#}
# plugin_output=1 gives more info than we need. We only use the first 4 fields
# of baseList output. Changing to 0.
# hostname  hack: http://nagios.measurementlab.net/baseList?show_state=1& \
#service_name="${plugin}"&plugin_output=0&show_problem_acknowledged=1
    curl -s "${nagios_auth}" -o "${SSH_OUTAGE}"/all_nodes_"${plugin}" --digest --netrc \
"http://nagios.measurementlab.net/baseList?show_state=1&service_name="${plugin}"&plugin_output=0&show_problem_acknowledged=1"
  fi
done
}

########################################
# Function: find_reboot_candidates
# Search the ssh and sshalt reports for nodes in state 2 (bad state),
# duration state 1 ("hard" state, meaning it's been that way for a while),
# and problem_acknowledged state 0 (not acknowledged).
# If there are nodes in state "2 1 0",
# creates files down_nodes_ssh and/or down_nodes_sshalt
# Find nodes in both the down ssh and sshalt lists
########################################
find_down_nodes() {

echo "Searching baseList output for nodes in hard state 1"
#for plugin in sshalt ssh; do
for plugin in "ssh" "sshalt"; do
  # Make the down_nodes file creation idempotent so we can append inside the
  # loop without getting unintended appended node lists from run to run
 if [ -f "${SSH_OUTAGE}/down_nodes_${plugin}" ]; then
  rm "${SSH_OUTAGE}/down_nodes_${plugin}"
fi  
  while read line; do
    host="$(echo $line | awk '{print $1 }')"
    state="$(echo $line | awk '{ print $2 }')"
    hard="$(echo $line | awk '{ print $3 }')"
    problem_acknowledged="$(echo $line | awk '{ print $4 }')"
if [[ ${state} == 2 ]] && [[ ${hard} == 1 ]] && \
    [[ "$problem_acknowledged" == 0 ]]; then
      echo "${host}" |grep -v ^s| awk -F. '{ print $1"."$2 }' \
      >> "${SSH_OUTAGE}/down_nodes_${plugin}"
      echo "${host}" |grep ^s | awk -F. '{ print $1"."$2 }' \
      >> "${SSH_OUTAGE}/down_switches_${plugin}"
    fi
      done < "${SSH_OUTAGE}/all_nodes_${plugin}"
  done
if [[ -s "${DOWN_NODES_SSH}" ]] && [[ -s "${DOWN_NODES_SSHALT}" ]]; then
  echo "There are down nodes to check; continuing"
  else echo "There are no down nodes; exiting"
  exit
fi
}

########################################
# Function: strip out the nodes with a sitename that matches
# the sitename of any down switch
########################################
find_reboot_candidates() {
# If both ssh and sshalt are both down, they go on the "maybe" list
comm --nocheck-order -12 "${DOWN_NODES_SSH}" "${DOWN_NODES_SSHALT}" > \
"${REBOOT_CANDIDATES}"

# The sshalt plugin doesn't report switches, but this file is a risidual of the
# loop above and isn't needed.
if [ -f "${SSH_OUTAGE}/down_switches_sshalt" ]; then
  rm "${SSH_OUTAGE}/down_switches_sshalt"
fi

if [[ -s "${DOWN_SWITCHES}" ]] ; then
  cp "${REBOOT_CANDIDATES}" "${REBOOT_ME}"
  echo "Down switches:"
  cat "${DOWN_SWITCHES}"
  for line in `cat "${DOWN_SWITCHES}" | awk -F. '{ print $2 }'`; do
#    echo "Stripping $line out of reboot_candidates"
   grep -v $line "${REBOOT_ME}" > "${REBOOT_ME}".tmp
    mv  "${REBOOT_ME}".tmp "${REBOOT_ME}"
  done
else
  echo "No down switches"
  cat "${REBOOT_CANDIDATES}" > "${REBOOT_ME}"
fi ;

if [[ -s "${REBOOT_ME}" ]] ; then
  echo "Please reboot these:"
  cat "${REBOOT_ME}"
else
  echo "No machines to reboot."
fi ;
}

########################################
# Run the functions
########################################
find_all_nodes
find_down_nodes
find_reboot_candidates
