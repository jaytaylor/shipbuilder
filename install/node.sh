#!/usr/bin/env bash

cd "$(dirname "$0")"

source libfns.sh

while getopts "d:f:H:S:h" OPTION; do
    case $OPTION in
        h)
            echo "usage: $0 -H [node-host] -S [shipbuilder-host] -d [device] -f [lxc-filesystem] [ACTION]" 1>&2
            echo '' 1>&2
            echo 'This is the node installer.' 1>&2
            echo '' 1>&2
            echo '  ACTION                Action to perform. Available actions are: list-devices, install'
            echo '  -S [shipbuilder-host] ShipBuilder server user@hostname (flag can be omitted if auto-detected from env/SB_SSH_HOST)' 1>&2
            echo '  -H [node-host]        Node user@hostname' 1>&2
            echo '  -d [device]           Device to install filesystem on' 1>&2
            echo '  -f [lxc-filesystem]   LXC filesystem to use; "zfs" or "btrfs" (flag can be ommitted if auto-detected from env/LXC_FS)' 1>&2
            exit 1
            ;;
        d)
            device=$OPTARG
            ;;
        f)
            lxcFs=$OPTARG
            ;;
        H)
            nodeHost=$OPTARG
            ;;
        S)
            sbHost=$OPTARG
            ;;
    esac
done

# Clear options from $n.
shift $(($OPTIND - 1))

action=$1

test -z "${sbHost}" && autoDetectServer
test -z "${lxcFs}" && autoDetectFilesystem

# Validate required parameters.
test -z "${sbHost}" && echo 'error: missing required parameter: -S [shipbuilder-host]' 1>&2 && exit 1
test -z "${nodeHost}" && echo 'error: missing required parameter: -H [node-host]' 1>&2 && exit 1
#test -z "${action}" && echo 'error: missing required parameter: action' 1>&2 && exit 1
if test -z "${action}"; then
    echo 'info: action defaulted to: install'
    action='install'
fi


verifySshAndSudoForHosts "${sbHost} ${nodeHost}" 


if [ "${action}" = "list-devices" ]; then
    echo '----'
	ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $nodeHost 'sudo find /dev/ -regex ".*\/\(\([hms]\|xv\)d\|disk\).*"'
    abortIfNonZero $? "retrieving storage devices from host ${sbHost}"
	exit 0

elif [ "${action}" = "install" ]; then
    test -z "${device}" && echo 'error: missing required parameter: -d [device]' 1>&2 && exit 1
    test -z "${lxcFs}" && echo 'error: missing required parameter: -f [lxc-filesystem]' 1>&2 && exit 1

    installAccessForSshHost $nodeHost
    
    rsync -azve "ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no'" libfns.sh $nodeHost:/tmp/
    abortIfNonZero $? 'rsync libfns.sh failed'

    ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $nodeHost "source /tmp/libfns.sh && prepareNode ${device} ${lxcFs}"
    abortIfNonZero $? 'remote prepareNode() invocation'

else
	echo 'unrecognized action: ${action}' 1>&2 && exit 1
fi


