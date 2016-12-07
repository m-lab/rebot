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
# TODO: Add tests for null response, from baseList and anywhere else that it matters

TIMESTAMP="$(date -u +%F_%H-%M)"
EPOCH_NOW="$(date -u +%s)"
EPOCH_YESTERDAY="$(date -u +%s --date='24 hours ago')"
#REBOT_LOG_DIR="/tmp/rebot"
REBOT_LOG_DIR="/home/steph/work/OTI/m-lab/git/salarcon215/rebot/logs/rebot-testing"
SSH_OUTAGE_TEMP_DIR="${REBOT_LOG_DIR}/ssh_outage"
REBOOT_HISTORY_DIR="${REBOT_LOG_DIR}/reboot_history"
ALL_HOSTS_SSH="${SSH_OUTAGE_TEMP_DIR}/all_hosts_ssh"
ALL_HOSTS_SSHALT="${SSH_OUTAGE_TEMP_DIR}/all_hosts_sshalt"
DOWN_NODES_SSH="${SSH_OUTAGE_TEMP_DIR}/down_hosts_ssh"
DOWN_NODES_SSHALT="${SSH_OUTAGE_TEMP_DIR}/down_hosts_sshalt"
DOWN_SWITCHES="${SSH_OUTAGE_TEMP_DIR}/down_switches_ssh"
REBOOT_CANDIDATES="${SSH_OUTAGE_TEMP_DIR}/reboot_candidates"
REBOOT_CANDIDATES_1="${SSH_OUTAGE_TEMP_DIR}/reboot_candidates.1"
REBOOT_ME="${SSH_OUTAGE_TEMP_DIR}/reboot_me"
NOTIFICATION_EMAIL="${SSH_OUTAGE_TEMP_DIR}/rebot_notify.out"
REBOOT_ATTEMPTED="${REBOOT_HISTORY_DIR}/reboot_attempted"
REBOOT_LOG="${REBOOT_HISTORY_DIR}/reboot_log"
PROBLEMATIC="${REBOOT_HISTORY_DIR}/problematic"
########################################
# Function: fresh_dirs
# Make a fresh SSH_OUTAGE_TEMP_DIR
# Make a persistent REBOOT_HISTORY_DIR if it doesn't already exist
########################################
fresh_dirs() {
  echo "#### Starting fresh_dirs(). Resetting the output dir. ####"

  rm -r "${SSH_OUTAGE_TEMP_DIR}"
  mkdir -p "${SSH_OUTAGE_TEMP_DIR}"
  echo "This directory gets recreated with fresh status files every time \
    rebot runs." > "${SSH_OUTAGE_TEMP_DIR}/README"

  if [ ! -d "${REBOOT_HISTORY_DIR}" ]; then
    mkdir "${REBOOT_HISTORY_DIR}"
    echo "This is a persistent directory that holds historical reboot \
      information." > "${REBOOT_HISTORY_DIR}/README"
  fi
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

  curl -s "${nagios_auth}" -o "${SSH_OUTAGE_TEMP_DIR}"/all_hosts_"$1" --digest --netrc \
    "http://nagios.measurementlab.net/baseList?show_state=1&service_name="$1"&plugin_output=0&show_problem_acknowledged=1"

  # TODO: Make this a unit test
  if [[ -s "${SSH_OUTAGE_TEMP_DIR}"/all_hosts_"$1" ]] ; then
  #  echo "${SSH_OUTAGE_TEMP_DIR}"/all_hosts_"$1" exists
    true
  else
    echo "No output of $1 ${ALL_HOSTS_}$1. That shouldn't happen. It's possible \
      the connection to the Nagios server failed."
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
########################################
find_down_hosts() {
  echo "#### Starting find_down_hosts(), hosts in hard state 1 for $1 ####"

  # Make the down_hosts file creation idempotent to avoid unintentional appends
  if [ -f "${SSH_OUTAGE_TEMP_DIR}/down_hosts_${1}" ]; then
    rm "${SSH_OUTAGE_TEMP_DIR}/down_hosts_${1}"
  fi

  if [ -f "${SSH_OUTAGE_TEMP_DIR}/down_switches_${1}" ]; then
    rm "${SSH_OUTAGE_TEMP_DIR}/down_switches_${1}"
  fi

  while read line; do
    host="$(echo $line | awk '{print $1 }')"
    state="$(echo $line | awk '{ print $2 }')"
    hard="$(echo $line | awk '{ print $3 }')"
    problem_acknowledged="$(echo $line | awk '{ print $4 }')"
    if [[ ${state} == 2 ]] && [[ ${hard} == 1 ]] && \
      [[ "$problem_acknowledged" == 0 ]]; then
      echo "${host}" |grep -v ^s| awk -F. '{ print $1"."$2 }' \
	>> "${SSH_OUTAGE_TEMP_DIR}/down_hosts_${1}"
      echo "${host}" |grep ^s | awk -F. '{ print $1"."$2 }' \
	>> "${SSH_OUTAGE_TEMP_DIR}/down_switches_${1}"
    fi
  done < "${SSH_OUTAGE_TEMP_DIR}/all_hosts_${1}"

}

########################################
# Function: no_down_nodes
# Exit here if there are no nodes reporting down.
# Supporting "Blue skies" as an M-Lab standard for reporting good news. :-)
# Inputs: DOWN_NODES_SSH, DOWN_NODES_SSHALT
########################################
no_down_nodes() {
  echo "#### Starting no_down_nodes() ####"

  if [[ -s "${DOWN_NODES_SSH}" ]] && [[ -s "${DOWN_NODES_SSHALT}" ]]; then
    # TODO: unit test: echo "There are down hosts to check; continuing"
    true
  else echo "Blue skies: There are no down hosts; exiting"
    exit
  fi

}

########################################
# Function: find_reboot_candidates
# Find nodes that are down according to both ssh and sshalt checks.
# Strip out hosts associated with a down switch
# Inputs: DOWN_NODES_SSH, DOWN_NODES_SSHALT, DOWN_SWITCHES
# Output: REBOOT_CANDIDATES
########################################
find_reboot_candidates() {
  echo "#### Starting find_reboot_candidates() ####"

  # If a node's ssh and sshalt statuses are both down, it's a candidate for reboot
  comm -12 <( sort "${DOWN_NODES_SSH}") <( sort "${DOWN_NODES_SSHALT}" ) > \
    "${REBOOT_CANDIDATES}"

  # TODO: unit test to make sure $REBOOT_CANDIDATES exists
  # echo "Contents of REBOOT_CANDIDATES after comparison of ssh and sshalt:"
  # cat "${REBOOT_CANDIDATES}"
  # echo ""

  # TODO: email interested parties. "Interested parties" seems like a candidate
  # for an M-Lab env var/M-Lab module.
  #echo "Now looking for down switches and removing them from REBOOT_CANDIDATES"
  if [[ -s "${DOWN_SWITCHES}" ]] ; then
    echo "Down switches! This shouldn't happen; please investigate and ask site
      partners to reboot the switch if needed." > ${NOTIFICATION_EMAIL}
    #cat ${NOTIFICATION_EMAIL}
    echo ""
    for switch in `cat "${DOWN_SWITCHES}" | awk -F. '{ print $2 }'`; do
      grep -v $switch "${REBOOT_CANDIDATES}" > "${REBOOT_CANDIDATES}".tmp
      mv  "${REBOOT_CANDIDATES}".tmp "${REBOOT_CANDIDATES}"
    done
  fi ;

  echo "Switch-free Contents of REBOOT_CANDIDATES:"
  if [[ -s "${REBOOT_CANDIDATES}" ]] ; then
    cat "${REBOOT_CANDIDATES}"
    echo ""
  else echo "Blue skies: There are no new reboot candidates."
  fi ;
}

########################################
# Function: did_they_come_back
# If a host from the previous attempt is still a reboot candidate, pull it out
# of reboot candidates and into a still_down file.
# If all hosts from the previous attempt are clean, scrub it from the
# reboot_attempted file, because it succeeded.
# Input: Reads from REBOOT_ATTEMPTED and REBOOT_CANDIDATES
# Output PROBLEMATIC 
# TODO: Reboot_log spreadsheet ID will be 1SnGHGy_ccvr4R5-_Pzn5bHr5Y986F9AT_v_GeFbgXto
# https://docs.google.com/spreadsheets/d/1SnGHGy_ccvr4R5-_Pzn5bHr5Y986F9AT_v_GeFbgXto/edit#gid=0
# example in mlabops/node-update/check_pipeline.py
# https://developers.google.com/sheets/guides/values
# https://developers.google.com/sheets/reference/rest/v4/spreadsheets.values
# TODO: (python rewrite) Test whether contents of PROBLEMATIC and REBOOT_LOG end up correct.
########################################
did_they_come_back() {
  echo "#### Starting did_they_come_back() ####"

#  echo "Contents of REBOOT_ATTEMPTED:"
#  cat ${REBOOT_ATTEMPTED}
#  echo ""
#  echo "Contents of REBOOT_CANDIDATES:"
#  cat ${REBOOT_CANDIDATES}
#  echo ""
#  echo "Contents of REBOOT_LOG:"
#  cat ${REBOOT_LOG}
#  echo ""

  for line in `cat "${REBOOT_ATTEMPTED}"`; do
    attempted_host=`echo $line | awk -F: '{ print $1 }'`
    if grep -q "${attempted_host}" "${REBOOT_CANDIDATES}" ; then
      # echo "Logging "${attempted_host}" in PROBLEMATIC for failed reboot."
      echo "${attempted_host} still not up but requested during last run:" \
        $line |tee -a ${PROBLEMATIC} "${NOTIFICATION_EMAIL}" > /dev/null
    else
      # echo "Logging $attempted_host in REBOOT_LOG because its reboot succeeded."
      echo $line >> "${REBOOT_LOG}"
    fi
  done


#  echo "New Contents of REBOOT_LOG:"
#  cat ${REBOOT_LOG}
#  echo ""
}

##########################
did_they_omg_comments() {
  echo "#### Starting did_they_come_back() ####"

  echo "Contents of REBOOT_ATTEMPTED:"
  cat ${REBOOT_ATTEMPTED}
  echo ""
  echo "Contents of REBOOT_CANDIDATES:"
  cat ${REBOOT_CANDIDATES}
  echo ""

  # rm -f "${REBOOT_ATTEMPTED}".tmp && touch "${REBOOT_ATTEMPTED}".tmp
  for line in `cat "${REBOOT_ATTEMPTED}"`; do
    echo "Line is:" $line
    # for host in `cat "${REBOOT_ATTEMPTED}" | awk -F: '{ print $1 }'`; do
    for attempted_host in `echo $line | awk -F: '{ print $1 }'`; do
      echo "Host is:" $attempted_host
      if grep -q $attempted_host "${REBOOT_CANDIDATES}" ; then
      #if grep -q `echo $line | awk -F: '{ print $1 }'` "${REBOOT_CANDIDATES}" ; then
	# cp "${REBOOT_CANDIDATES}" "${REBOOT_CANDIDATES}".$TIMESTAMP
	# echo "$attempted_host attempted to boot during the last run but is still down.
	#  Storing in Problematic and scrubbing from REBOOT_CANDIDATES."
	echo $attempted_host" still not up but requested during last run:" $line |tee $PROBLEMATIC >>"${NOTIFICATION_EMAIL}"
	#grep -v $attempted_host "${REBOOT_CANDIDATES}" > "${REBOOT_CANDIDATES}".tmp
	#mv  "${REBOOT_CANDIDATES}".tmp "${REBOOT_CANDIDATES}"

      else
	echo "Logging $attempted_host in REBOOT_LOG and scrubbing from
	  REBOOT_ATTEMPTED because its reboot succeeded."
	echo $line >> "${REBOOT_LOG}"
	#cp "${REBOOT_ATTEMPTED}" "${REBOOT_ATTEMPTED}".$TIMESTAMP
	#grep -v $host "${REBOOT_ATTEMPTED}" > "${REBOOT_ATTEMPTED}".tmp
	#mv  "${REBOOT_ATTEMPTED}".tmp "${REBOOT_ATTEMPTED}"

	fi
      done
  done
  #echo "New Contents of REBOOT_ATTEMPTED (/tmp/rebot-testing/reboot_history/reboot_attempted):"
  #cat ${REBOOT_ATTEMPTED}
  #echo ""
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

  #echo "Contents of REBOOT_CANDIDATES (/tmp/rebot-testing/ssh_outage/reboot_candidates:"
  cat ${REBOOT_CANDIDATES}
  #echo "Contents of REBOOT_LOG (/tmp/rebot-testing/reboot_history/reboot_log:"
  cat ${REBOOT_LOG}
  #echo "# Starting timestamp comparison #"
  #echo ""

  rm -f "${REBOOT_CANDIDATES}.tmp" && touch "${REBOOT_CANDIDATES}.tmp"
  for host in `cat "${REBOOT_CANDIDATES}"`; do
    if grep -q $host "${REBOOT_LOG}" ; then
      # echo ""
      # echo "$host found in REBOOT_LOG. Checking its timestamp to see if it's been more than 24 hours."
      PREVIOUS_REBOOT=`grep $host $REBOOT_LOG | head -1 | awk -F: '{print $3 }'`
      # echo "Previous reboot:" $PREVIOUS_REBOOT
      SECONDS_SINCE_REBOOT=$(($EPOCH_NOW - $PREVIOUS_REBOOT ))
      # echo "$host previous reboot:" $PREVIOUS_REBOOT". Seconds since: "$SECONDS_SINCE_REBOOT". Should be more than 86400."
      if [ "${SECONDS_SINCE_REBOOT}" -gt 86400 ]; then
        # echo "More than a day since $host was rebooted. Ok to reboot. Echoing into REBOOT_CANDIDATES"
        echo $host >> "${REBOOT_CANDIDATES}.tmp"
      else
        # echo "Less than a day since $host was rebooted. Not ok to reboot."
        echo "Less than a day since $host was rebooted (${SECONDS_SINCE_REBOOT}" \
          "seconds. Should be more than 86400.) Not ok to reboot." >> ${NOTIFICATION_EMAIL}
      fi
    else
      #echo "$host is not present in the REBOOT_LOG. Ok to proceed."
      echo $host >> "${REBOOT_CANDIDATES}.tmp"
    fi
  done

mv "${REBOOT_CANDIDATES}.tmp" "${REBOOT_CANDIDATES}"

#  echo ""
#  echo "New Contents of REBOOT_CANDIDATES (/tmp/rebot-testing/ssh_outage/reboot_candidates:"
#  cat ${REBOOT_CANDIDATES}
#  echo ""
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
  # make a fresh copy of ${REBOOT_ATTEMPTED}. Any relevant entries from the previous run are processed into REBOOT_LOG or $PROBLEMATIC in the did_they_come_back() function.
  rm -f "${REBOOT_ATTEMPTED}" && touch "${REBOOT_ATTEMPTED}"
  if [[ -s "${REBOOT_ME}" ]] ; then
    if [[ `cat "${REBOOT_ME}" | wc -l` -gt 5 ]] ; then
      echo "There are more than 5 hosts queued for reboot. This is unusual" \
        "and could be dangerous! Please check the fleet for problems. " \
        "Exiting." >> ${NOTIFICATION_EMAIL}
    else
      # TODO: call drac.py to do this automatically
      echo "Please reboot these:"
      cat "${REBOOT_ME}"
      for line in `cat "${REBOOT_ME}"`; do
        echo $line":"$TIMESTAMP":"$EPOCH_NOW >> "${REBOOT_ATTEMPTED}"
      done
    fi
  else
   echo "No machines to reboot."
  fi ;
}

########################################
# Notify 
########################################
notify() {
  mail -s "Notification from ReBot" salarcon@opentechinstitute.org < ${NOTIFICATION_EMAIL}
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
# make a --test flag
# set up some known inputs

#fresh_dirs
#find_all_hosts ssh
#find_all_hosts sshalt

#find_down_hosts ssh
#find_down_hosts sshalt
#no_down_nodes
#find_reboot_candidates
did_they_come_back
#has_it_been_24_hrs
#perform_the_reboot
quit
notify
