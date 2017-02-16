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
# TODO (python rewrite): Add tests for null response, from baseList and anywhere else that it matters

TIMESTAMP="$(date -u +%F_%H-%M)"
EPOCH_NOW="$(date -u +%s)"
EPOCH_YESTERDAY="$(date -u +%s --date='24 hours ago')"
REBOT_LOG_DIR="/var/log/rebot"
#REBOT_LOG_DIR="/home/steph/work/OTI/m-lab/git/salarcon215/rebot/logs/rebot-testing"
SSH_OUTAGE_TEMP_DIR="${REBOT_LOG_DIR}/ssh_outage"
REBOOT_HISTORY_DIR="${REBOT_LOG_DIR}/reboot_history"
ALL_HOSTS_SSH="${SSH_OUTAGE_TEMP_DIR}/all_hosts_ssh"
ALL_HOSTS_SSHALT="${SSH_OUTAGE_TEMP_DIR}/all_hosts_sshalt"
DOWN_HOSTS_SSH="${SSH_OUTAGE_TEMP_DIR}/down_hosts_ssh"
DOWN_HOSTS_SSHALT="${SSH_OUTAGE_TEMP_DIR}/down_hosts_sshalt"
DOWN_SWITCHES="${SSH_OUTAGE_TEMP_DIR}/down_switches_ssh"
REBOOT_CANDIDATES="${SSH_OUTAGE_TEMP_DIR}/reboot_candidates"
NOTIFICATION_EMAIL="${SSH_OUTAGE_TEMP_DIR}/rebot_notify.out"
REBOOT_ATTEMPTED="${REBOOT_HISTORY_DIR}/reboot_attempted"
REBOOT_LOG="${REBOOT_HISTORY_DIR}/reboot_log"
PROBLEMATIC="${REBOOT_HISTORY_DIR}/problematic"
TOOLS_DIR="/home/salarcon/git/operator/tools"
########################################
# Function: fresh_dirs
# Make a fresh SSH_OUTAGE_TEMP_DIR
# Make a persistent REBOOT_HISTORY_DIR if it doesn't already exist
########################################
fresh_dirs() {
  echo "#### Starting fresh_dirs(). Resetting the output dir. ####"

  if [ ! -d "${REBOOT_HISTORY_DIR}" ]; then
    mkdir -p "${REBOOT_HISTORY_DIR}"
    touch "${REBOOT_LOG}"
    echo "This is a persistent directory that holds historical reboot \
      information." > "${REBOOT_HISTORY_DIR}/README"
  fi

  rm -r "${SSH_OUTAGE_TEMP_DIR}" > /dev/null 2>&1
  mkdir -p "${SSH_OUTAGE_TEMP_DIR}"
  echo "This directory gets recreated with fresh status files every time \
    rebot runs." > "${SSH_OUTAGE_TEMP_DIR}/README"
}

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
  echo "#### Starting find_all_hosts(). Getting status of $1 from Nagios. ####"

  local service_name="${1}"
  local output_file="${2}"
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

  curl -s "${nagios_auth}" -o "${output_file}" --digest --netrc \
    "http://nagios.measurementlab.net/baseList?show_state=1&service_name="${service_name}"&plugin_output=0&show_problem_acknowledged=1"

  # TODO: Make this a unit test
  if [[ ! -s "${output_file}" ]] ; then
    echo "No output of $service_name $output_file. That shouldn't happen. It's " \
      "possible the connection to the Nagios server failed."
    echo ""
  fi ;
}


########################################
# Function: find_down_hosts
# Search the ssh and sshalt reports for hosts in state 2 (bad state),
# duration state 1 ("hard" state, meaning it's been that way for a while),
# and problem_acknowledged state 0 (not acknowledged).
# If there are hosts in state "2 1 0",
# create files DOWN_HOSTS_SSH and/or DOWN_HOSTS_SSHALT, and DOWN_SWITCHES_SSH
# Inputs: ALL_HOSTS_SSH, ALL_HOSTS_SSHALT
# Outputs: DOWN_HOSTS_SSH, DOWN_HOSTS_SSHALT, DOWN_SWITCHES_SSH
# find_down_hosts "^s1" "${ALL_HOSTS_SSH}" "${DOWN_SWITCHES}"
# find_down_hosts "^mlab[1-4]" "${ALL_HOSTS_SSH}" "${DOWN_HOSTS_SSH}"
# find_down_hosts "^mlab[1-4]" "${ALL_HOSTS_SSHALT}" "${DOWN_DOWN_HOSTS_SSHALT}"
########################################
find_down_hosts() {
  echo "#### Starting find_down_hosts() ####"
  local filter="${1}"
  local service_name="${2}"
  local output_file="${3}"

  while read line; do
    host="$(echo $line | awk '{print $1 }')"
    state="$(echo $line | awk '{ print $2 }')"
    hard="$(echo $line | awk '{ print $3 }')"
    problem_acknowledged="$(echo $line | awk '{ print $4 }')"
    if [[ ${state} == 2 ]] && [[ ${hard} == 1 ]] && \
      [[ "$problem_acknowledged" == 0 ]]; then
      echo "${host}" | grep "${filter}" | awk -F. '{ print $1"."$2 }' \
        >> "${output_file}"
    fi
  done < "${service_name}"

}

########################################
# Function: no_down_nodes
# Exit here if there are no nodes reporting down.
# Supporting "Blue skies" as an M-Lab standard for reporting good news. :-)
# Inputs: DOWN_HOSTS_SSH, DOWN_HOSTS_SSHALT
########################################
no_down_nodes() {
  echo "#### Starting no_down_hosts() ####"

  if [[ ! -s "${DOWN_HOSTS_SSH}" ]] && [[ ! -s "${DOWN_HOSTS_SSHALT}" ]]; then
    echo "Blue skies: There are no down hosts; exiting"
  else
    # TODO: unit test: echo "There are down hosts to check; continuing"
    true
  fi
}

########################################
# Function: find_reboot_candidates
# Find nodes that are down according to both ssh and sshalt checks.
# Strip out hosts associated with a down switch
# Inputs: DOWN_HOSTS_SSH, DOWN_HOSTS_SSHALT, DOWN_SWITCHES
# Output: REBOOT_CANDIDATES
########################################
find_reboot_candidates() {
  echo "#### Starting find_reboot_candidates() ####"

  # If a node's ssh and sshalt statuses are both down, it's a candidate for reboot
  comm -12 <( sort "${DOWN_HOSTS_SSH}") <( sort "${DOWN_HOSTS_SSHALT}" ) > \
    "${REBOOT_CANDIDATES}"

  # TODO: unit test to make sure $REBOOT_CANDIDATES exists
  rm -f "${REBOOT_CANDIDATES}.tmp" && touch "${REBOOT_CANDIDATES}.tmp"
  for line in `cat "${REBOOT_CANDIDATES}"`; do
    switch=`echo "${line}" | awk -F. '{ print $2 }'`
    if grep -q "${switch}" "${DOWN_SWITCHES}" ; then
      echo "The switch at "${switch}" is down. Please fix it." |tee -a \
        "${PROBLEMATIC}" "${NOTIFICATION_EMAIL}" > /dev/null
    else
      echo "${line}" >>  "${REBOOT_CANDIDATES}.tmp"
    fi
  done

  mv "${REBOOT_CANDIDATES}.tmp" "${REBOOT_CANDIDATES}"

  if [[ -s "${REBOOT_CANDIDATES}" ]] ; then
    echo "Switch-free Contents of REBOOT_CANDIDATES:"
    cat "${REBOOT_CANDIDATES}"
    echo ""
  else
    echo "Blue skies: There are no new reboot candidates."
    notify
  fi ;

}

########################################
# Function: did_they_come_back
# If a host from the previous attempt is still a reboot candidate, log it in 
# PROBLEMATIC and notify, but try again to reboot it.
# Otherwise, log successful reboots in the REBOOT_LOG.
# Input: Reads from REBOOT_ATTEMPTED and REBOOT_CANDIDATES
# Output: PROBLEMATIC and NOTIFICATION_EMAIL
# TODO: (python rewrite) Test whether contents of PROBLEMATIC and REBOOT_LOG end up correct.
# TODO: Reboot_log spreadsheet ID will be 1SnGHGy_ccvr4R5-_Pzn5bHr5Y986F9AT_v_GeFbgXto
# https://docs.google.com/spreadsheets/d/1SnGHGy_ccvr4R5-_Pzn5bHr5Y986F9AT_v_GeFbgXto/edit#gid=0
# example in mlabops/node-update/check_pipeline.py
# https://developers.google.com/sheets/guides/values
# https://developers.google.com/sheets/reference/rest/v4/spreadsheets.values
########################################
did_they_come_back() {
  echo "#### Starting did_they_come_back() ####"

  for line in `cat "${REBOOT_ATTEMPTED}"`; do
    attempted_host=`echo "${line}" | awk -F: '{ print $1 }'`
    if grep -q "${attempted_host}" "${REBOOT_CANDIDATES}" ; then
      echo "${attempted_host} reboot tried during last run but still down:" \
        "${line}" | tee -a "${PROBLEMATIC}" "${NOTIFICATION_EMAIL}" > /dev/null
    else
# TODO: The running log should be backed up somewhere. Google doc? Something else?
      echo "${line}" >> "${REBOOT_LOG}" && echo "${attempted_host} was rebooted" \
        "successfully during the last run." >> "${NOTIFICATION_EMAIL}"
    fi
  done
}

########################################
# Function: has_it_been_24_hrs() 
# Make sure the reboot_candidates weren't rebooted in the last 24 hours
# Take current epoch time, epoch time of previous entry for
# this server, subtract previous from current, make sure it's more than 86400s
# or 24 hours. SECONDS_IN_A_DAY=$(($EPOCH_NOW - $EPOCH_YESTERDAY))
# Input: REBOOT_CANDIDATES
# Output: REBOOT_CANDIDATES
########################################
has_it_been_24_hrs() {
  echo "#### Starting has_it_been_24_hrs ####"

  rm -f "${REBOOT_CANDIDATES}.tmp" && touch "${REBOOT_CANDIDATES}.tmp"
  for host in `cat "${REBOOT_CANDIDATES}"`; do
    if grep -q "${host}" "${REBOOT_LOG}" ; then
      PREVIOUS_REBOOT=`grep "${host}" "${REBOOT_LOG}" | tail -1 | awk -F: '{print $3 }'`
      SECONDS_SINCE_REBOOT=$((${EPOCH_NOW} - ${PREVIOUS_REBOOT} ))
      if [ "${SECONDS_SINCE_REBOOT}" -gt 86400 ]; then
        echo "${host}" >> "${REBOOT_CANDIDATES}.tmp"
      else
        echo "Less than a day since $host was rebooted (${SECONDS_SINCE_REBOOT}" \
          "seconds. Should be more than 86400.) Not ok to reboot." \
            | tee -a "${PROBLEMATIC}" "${NOTIFICATION_EMAIL}" > /dev/null
      fi
    else
      echo "${host}" >> "${REBOOT_CANDIDATES}.tmp"
    fi
  done
  cp "${REBOOT_CANDIDATES}.tmp" "${REBOOT_CANDIDATES}"
}

########################################
# Function: perform_the_reboot
# At this point, the REBOOT_ME list should be clean
# Need to make sure only 5 candidates get rebooted at a time
# and we can perform the drac command and dump it to the attempted list
# Takes REBOOT_ME as input, write to REBOOT_ATTEMPTED
########################################
perform_the_reboot() {
  echo "#### Starting perform_the_reboot ####"

  # Make a fresh copy of ${REBOOT_ATTEMPTED}. Any relevant entries from the
  # previous run are processed into REBOOT_LOG or PROBLEMATIC in the
  # function did_they_come_back().
  rm -f "${REBOOT_ATTEMPTED}" && touch "${REBOOT_ATTEMPTED}"
  if [[ -s "${REBOOT_CANDIDATES}" ]] ; then
    if [[ `cat "${REBOOT_CANDIDATES}" | wc -l` -gt 5 ]] ; then
      echo "There are more than 5 hosts queued for reboot. This is unusual" \
        "and could be dangerous! Please check the fleet for problems." \
        | tee -a ${PROBLEMATIC} "${NOTIFICATION_EMAIL}" > /dev/null
    else
      for line in `cat "${REBOOT_CANDIDATES}"`; do
        host="$(echo $line | awk -F: '{print $1 }')"
        echo $line":"$TIMESTAMP":"$EPOCH_NOW >> "${REBOOT_ATTEMPTED}"
       $TOOLS_DIR/drac.py reboot ${host} 
      done
    fi
  else
   echo "No machines to reboot."
  fi ;
}

########################################
# Notify
# TODO: email interested parties. "Interested parties" seems like a candidate
# for an M-Lab env var or module.
# TODO: Separate notifications so that problems get sent right away (down
# switch) but reboot notices get collected and sent daily.
########################################
notify() {
  if [[ -s "${NOTIFICATION_EMAIL}" ]] ; then
    mail -s "Notification from ReBot" salarcon@opentechinstitute.org \
      < ${NOTIFICATION_EMAIL}
  fi
}

########################################
# FIN
########################################
quit() {
  exit
}


########################################
# Run the functions
########################################
# TODO: make a --test flag and create some known inputs
# TODO: make a --dryrun flag that shows reboot candidates but doesn't act

fresh_dirs
find_all_hosts ssh "${ALL_HOSTS_SSH}"
find_all_hosts sshalt "${ALL_HOSTS_SSHALT}"
find_down_hosts "^s1" "${ALL_HOSTS_SSH}" "${DOWN_SWITCHES}"
find_down_hosts "^mlab[1-4]" "${ALL_HOSTS_SSH}" "${DOWN_HOSTS_SSH}"
find_down_hosts "^mlab[1-4]" "${ALL_HOSTS_SSHALT}" "${DOWN_HOSTS_SSHALT}"
no_down_nodes
find_reboot_candidates
did_they_come_back
has_it_been_24_hrs
perform_the_reboot
notify
quit
