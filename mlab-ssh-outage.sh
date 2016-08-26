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
SSH_OUTAGE=/tmp/rebot/ssh_outage
DOWN_NODES_SSHALT=${SSH_OUTAGE}/down_nodes_sshalt
DOWN_NODES_SSH=${SSH_OUTAGE}/down_nodes_ssh
DOWN_SWITCHES=${SSH_OUTAGE}/down_switches_ssh
REBOOT_CANDIDATES=${SSH_OUTAGE}/reboot_candidates
REBOOT_ME=${SSH_OUTAGE}/reboot_me

########################################
# Function: ask_baselist
# Authenticate to Nagios to view baseList.pl output for ssh and sshalt
########################################
ask_baselist() {

rm -r ${SSH_OUTAGE}
mkdir -p ${SSH_OUTAGE}

# If the script is running on eb, run baseList locally. 
# Anywhere else, use the URL of the cgi script on the Nagios server. 
echo "Getting status of ssh and sshalt from Nagios"
for plugin in sshalt ssh; do
  if [[ $(hostname -f) = "eb.measurementlab.net" ]] ; then
    sudo /etc/nagios3/baseList.pl show_state=1 service_name=${plugin} \
plugin_output=0 show_problem_acknowledged=1 >> \
${SSH_OUTAGE}/mlab_nodes_${plugin}
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
    curl -s $nagios_auth -o ${SSH_OUTAGE}/mlab_nodes_${plugin} --digest --netrc \
"http://nagios.measurementlab.net/baseList?show_state=1&service_name=${plugin}&plugin_output=1&show_problem_acknowledged=1"
  fi
done
}

########################################
# Function: find_reboot_candidates
# Search the ssh and sshalt reports for nodes in state 2 (bad state),
# duration state 2 ("hard" state, meaning it's been that way for a while),
# and problem_acknowledged state 0 (not acknowledged).
# Find nodes in both the down ssh and sshalt lists
########################################

find_reboot_candidates() {

echo "Searching baseList output for nodes in hard state 2"
for plugin in sshalt ssh; do
  while read line; do
    host=$(echo $line | awk '{print $1 }')
    state=$(echo $line | awk '{ print $2 }')
    hard=$(echo $line | awk '{ print $3 }')
    problem_acknowledged=$(echo $line | awk '{ print $4 }')

    if [[ $state == 2 ]] && [[ $hard == 1 ]] && \
    [[ $problem_acknowledged == 0 ]]; then
      echo $host |grep -v ^s| awk -F. '{ print $1"."$2 }' \
      >> $SSH_OUTAGE/down_nodes_${plugin}
      echo $host |grep ^s | awk -F. '{ print $1"."$2 }' \
      >> $SSH_OUTAGE/down_switches_${plugin}
    fi
      done < "${SSH_OUTAGE}/mlab_nodes_${plugin}"
  done

comm --nocheck-order -12 ${DOWN_NODES_SSH} ${DOWN_NODES_SSHALT} > \
${REBOOT_CANDIDATES}
}

########################################
# Function: strip out the nodes with a sitename that matches
# the sitename of any down switch
########################################

remove_down_switches() {
if [ -f $SSH_OUTAGE/down_switches_sshalt ]; then
  rm $SSH_OUTAGE/down_switches_sshalt
fi

if [[ -s ${DOWN_SWITCHES} ]] ; then
  cp ${REBOOT_CANDIDATES} ${REBOOT_ME}
  echo "Down switches:"
  cat ${DOWN_SWITCHES}
  for line in `cat ${DOWN_SWITCHES} | awk -F. '{ print $2 }'`; do
#    echo "Stripping $line out of reboot_candidates"
   grep -v $line ${REBOOT_ME} > ${REBOOT_ME}.tmp
    mv  ${REBOOT_ME}.tmp ${REBOOT_ME}
  done
else
  echo "No down switches"
  cat ${REBOOT_CANDIDATES} > ${REBOOT_ME}
fi ;

if [[ -s ${REBOOT_ME} ]] ; then
  echo "Please reboot these:"
  cat ${REBOOT_ME}
else
  echo "No machines to reboot."
fi ;
}

########################################
# Run the functions
########################################
ask_baselist
find_reboot_candidates
remove_down_switches
