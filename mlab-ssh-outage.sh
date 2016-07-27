#!/bin/bash 

rm -r sshalt_outage*

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

echo "Creating the file /tmp/mlab-nodes from Nagios sshalt plugin"
curl -s $nagios_auth -o /tmp/mlab-nodes-sshalt --digest --netrc "http://nagios.measurementlab.net/baseList?show_state=1&service_name=sshalt&plugin_output=1"
curl -s $nagios_auth -o /tmp/mlab-nodes-ssh --digest --netrc "http://nagios.measurementlab.net/baseList?show_state=1&service_name=ssh&plugin_output=1"

full_host=1
if [ "$1" == "-v" ]
then
	full_host=0
	echo "Full_host value is " $full_host
	shift
fi

timestamp="$(date +%F_%H-%M)"
mkdir sshalt_outage_${timestamp}
outage="sshalt_outage_${timestamp}/sshalt_down_nodes.txt"
echo "Searching /tmp/mlab-nodes-sshalt for nodes in state 2"
while read line
do
#    node=$(echo $line | sed "s/\(mlab[1-4]\.[a-z]\{3\}[0-9]\{2\}\).*/\1/")
#    echo "Node is $node".
	host=$(echo $line | awk '{print $1 }')
	state=$(echo $line | awk '{ print $2 }')
	hard=$(echo $line | awk '{ print $3 }')

	if [ $state == 2 ] && [ $hard == 1 ]
	then
		echo "State of $host is $state and is in hard down state." >> $outage
		echo $line >> $outage	
		continue
	fi
#    echo $line | grep "CRITICAL" 1>/dev/null 2>&1
#    if [ $? -ne 0 ]
#    then
#        continue
#    fi
#    ssh -n $node.measurement-lab.org "traceroute eb.measurementlab.net" > outage_${timestamp}/$node-ks.traceroute &
#	echo $node >> $outage
done < "/tmp/mlab-nodes-sshalt"

echo "Searching /tmp/mlab-nodes-ssh for nodes in state 2"
while read line
do
	host=$(echo $line | awk '{print $1 }')
	state=$(echo $line | awk '{ print $2 }')
	hard=$(echo $line | awk '{ print $3 }')

	if [ $state == 2 ] && [ $hard == 1 ]
	then
		echo "State of $host is $state and is in hard down state." >> $outage
		echo $line >> $outage	
		continue
	fi
done < "/tmp/mlab-nodes-ssh"
