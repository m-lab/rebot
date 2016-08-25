#!/bin/bash
#
# Reboot crashed nodes.
# TODO: Do appropriate var nams have curly braces?
# TODO: Do all repeated paths/files have working vars associated?
# TODO: If hostname is eb, then is it running as root? If not bail out because baselist runs locally as root
# TODO: Find out if baselist really has to be only root-executable. Without that, this wouldn't need to be run by root.
# TODO: Are all sections that should be functions written as functions?
# TODO: Get us to the right directory at the beginning of the script (all in /tmp/rebot?) so the script doesn't litter the system with junk files

TIMESTAMP="$(date +%F_%H-%M)"
#SSH_OUTAGE=ssh_outage_${TIMESTAMP}
SSH_OUTAGE=ssh_outage
sshalt_nodes=${SSH_OUTAGE}/mlab_nodes_sshalt
ssh_nodes=${SSH_OUTAGE}/mlab_nodes_ssh
down_nodes_sshalt=${SSH_OUTAGE}/down_nodes_sshalt
down_nodes_ssh=${SSH_OUTAGE}/down_nodes_ssh
down_switches=$SSH_OUTAGE/down_switches_ssh
reboot_candidates=$SSH_OUTAGE/reboot_candidates
REBOOT_ME=${SSH_OUTAGE}/reboot_me

#####################
# Make this a function and make it conditional on being un on eb
# Configure a user's login to Nagios.  For more information on netrc files, see
# the manpage for curl.  If a netrc file doesn't exist, then the user will be
# prompted to enter credentials.
#######################################
# Authenticate to Nagios to view baseList.pl output
# Globals:
# Arguments:
#   None
# Returns:
#   None
#######################################
# Function: Grab all results from Nagios for sshalt and ssh
ask_baselist () {

# Clean up previous results
rm -r ${SSH_OUTAGE}
mkdir ${SSH_OUTAGE}


for plugin in sshalt ssh; do
  echo "Creating mlab_nodes_$plugin from Nagios"
# if hostname is eb, then:
  if [[ $(hostname -f) = "eb.measurementlab.net" ]] ; then
    echo "We're on eb"
    /etc/nagios3/baseList.pl show_state=1 service_name=$plugin plugin_output=0 show_problem_acknowledged=1 >>  ${SSH_OUTAGE}/mlab_nodes_$plugin
  else
  #nagios_auth() {
      if [ -f ~/.netrc ] && grep -q 'nagios.measurementlab.net' ~/.netrc; then
      nagios_auth='--netrc'
      else
    read -p "Nagios login: " nagios_login
    read -s -p "Nagios password: " nagios_pass
    nagios_auth="--user ${nagios_login}:${nagios_pass}"
    echo -e '\n'
  fi
fi
#}
    curl -s $nagios_auth -o ${SSH_OUTAGE}/mlab_nodes_$plugin --digest --netrc \
"http://nagios.measurementlab.net/baseList?show_state=1&service_name=$plugin&plugin_output=1&show_problem_acknowledged=1"
done
}

#################################
# Function: find_reboot_candidates
# Search the ssh and sshalt reports for nodes in state 2 (bad state),
# duration state 2 ("hard" state, meaning it's been that way for a while),
# and problem_acknowledged state 0 (not acknowledged).
# Compare the lists of down ssh and sshalt hosts and find the
# commonalities

find_reboot_candidates () {
  #echo "Searching "${plugin}_nodes" for nodes in hard state 2"
for plugin in sshalt ssh; do
  echo "Searching "${SSH_OUTAGE}/mlab_nodes_$plugin" for nodes in hard state 2"
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
      done < "${SSH_OUTAGE}/mlab_nodes_$plugin"
done

comm --nocheck-order -12 ${down_nodes_ssh} ${down_nodes_sshalt} > ${reboot_candidates}
}

############
# Function: sed out the nodes with a sitename that matches the sitename of any
# down switch 
remove_down_switches () {
if [ -f $SSH_OUTAGE/down_switches_sshalt ]; then
  rm $SSH_OUTAGE/down_switches_sshalt
fi

if [[ -s ${down_switches} ]] ; then
  cp ${reboot_candidates} ${REBOOT_ME} 
  echo "Down switches:"
  cat ${down_switches}
  for line in `cat ${down_switches} | awk -F. '{ print $2 }'`; do
    echo "Stripping $line out of reboot_candidates" 
   grep -v $line ${REBOOT_ME} > ${REBOOT_ME}.tmp
    mv  ${REBOOT_ME}.tmp ${REBOOT_ME}
  done
else
  echo "No down switches"
  cat ${reboot_candidates} > ${REBOOT_ME}
fi ;

if [[ -s ${REBOOT_ME} ]] ; then
  echo "Please reboot these:"
  cat ${REBOOT_ME}
else
  echo "No machines to reboot."
fi ;
}
#cd ..
#for line in `cat reboot_me_$timestamp`; do

#find /tmp/rebot -mtime +1 -exec rm {} \;


# Declare yo functions
ask_baselist
#find_reboot_candidates
#remove_down_switches
