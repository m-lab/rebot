#!/bin/bash
#
# This script replaces drac.py for testing purposes.
#
# It takes two arguments and exits with an exit status of 1 if the machine
# name starts with mlab4.* 

# Make sure there are two arguments
if [ $# != 2 ] ; then
    echo $USAGE
    exit 1;
fi

# Fail if the machine name starts with mlab4.*
REGEX_HOST='mlab4.*'
if [[ $2 =~ $REGEX_HOST ]] ; then
    exit 1;
fi

exit 0