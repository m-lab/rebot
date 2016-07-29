#!/bin/bash 

rm -r ssh_outage*

timestamp="$(date +%F_%H-%M)"
mkdir ssh_outage_${timestamp}
ssh_outage=ssh_outage_${timestamp}
sshalt_down_nodes="ssh_outage_${timestamp}/sshalt_down_nodes.txt"
ssh_down_nodes="ssh_outage_${timestamp}/ssh_down_nodes.txt"

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

# Grab all results from Nagios for sshalt and ssh
for plugin in sshalt ssh
do
echo "Creating /tmp/mlab-nodes-$plugin from Nagios"
curl -s $nagios_auth -o /tmp/mlab-nodes-$plugin --digest --netrc "http://nagios.measurementlab.net/baseList?show_state=1&service_name=$plugin&plugin_output=1"

# Look for nodes in state 2 in baseList.pl, which indicates a bad state, 
# and state duration of 1, a "hard" state, meaning it's been that way for a while 

	echo "Searching /tmp/mlab-nodes-${plugin} for nodes in hard state 2"
	while read line
	do
		host=$(echo $line | awk '{print $1 }')
		state=$(echo $line | awk '{ print $2 }')
		hard=$(echo $line | awk '{ print $3 }')

		if [ $state == 2 ] && [ $hard == 1 ]
		then
			echo $host |grep -v ^s| awk -F. '{ print $1"."$2 }' >> $ssh_outage/${plugin}_down_nodes
			echo $host |grep ^s | awk -F. '{ print $1"."$2 }' >> $ssh_outage/${plugin}_down_switches
		fi
		done < "/tmp/mlab-nodes-${plugin}"
done
rm $ssh_outage/sshalt_down_switches
cd $ssh_outage
comm --nocheck-order -12 *nodes* > reboot_candidates
for line in `cat ssh_down_switches | awk -F. '{ print $2 }'`
do grep -v $line reboot_candidates > reboot_me
done
