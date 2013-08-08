#!/usr/bin/env bash

cd "$(dirname "$0")"

source libfns.sh

RESOURCES='server.sh setupBtrfs.sh loadBalancer.sh'

while getopts “d:S:h” OPTION; do
    case $OPTION in
        h)
            echo "usage: $0 -S [shipbuilder-host] -d [server-btrfs-device] ACTION" 1>&2
            echo '' 1>&2
            echo 'This is the ShipBuilder installer program.' 1>&2
            echo '' 1>&2
            echo '  ACTION                      Action to perform. Available actions are: install, list-devices'
            echo '  -S [shipbuilder-host]       ShipBuilder Server SSH address (e.g. ubuntu@my.sb)' 1>&2
            echo '  -d [sb-server-btrfs-device] Device to format with BTRFS (e.g. /dev/xvdc)' 1>&2
            exit 1
            ;;
        S)
            sbHost=$OPTARG
            ;;
        d)
            device=$OPTARG
            ;;
    esac
done

# Clear options from $n.
shift $(($OPTIND - 1))

action=$1

test -z "${sbHost}" && autoDetectServer

test -z "${sbHost}" && echo 'error: missing required parameter: -S [shipbuilder-host]' 1>&2 && exit 1
test -z "${action}" && echo 'error: missing required parameter: action' 1>&2 && exit 1


verifySshAndSudoForHosts "${sbHost}"


getIpCommand="ifconfig | tr '\t' ' '| sed 's/ \{1,\}/ /g' | grep '^e[a-z]\+0[: ]' --after 8 | grep --only 'inet \(addr:\)\?[: ]*[^: ]\+' | tr ':' ' ' | sed 's/\(.*\) addr[: ]\{0,\}\(.*\)/\1 \2/' | sed 's/ \{1,\}/ /g' | cut -f2 -d' '"


if [ "${action}" = "list-devices" ]; then
    echo '----'
    ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $sbHost 'sudo find /dev/ -regex ".*\/\(\([hms]\|xv\)d\|disk\).*"'
    abortIfNonZero $? "retrieving storage devices from host ${sbHost}"
    exit 0

elif [ "${action}" = "install" ]; then
    test -z "${device}" && echo 'error: missing required parameter: -d [device]' 1>&2 && exit 1

    installAccessForSshHost $sbHost

    rsync -azve "ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no'" libfns.sh $sbHost:/tmp/
    ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $sbHost "source /tmp/libfns.sh && prepareServerPart1 ${sbHost} ${device}"
    mv ../env/SB_SSH_HOST{,.bak}
    echo "${sbHost}" > ../env/SB_SSH_HOST
    ../deploy.sh -f
    mv ../env/SB_SSH_HOST{.bak,}
    ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $sbHost "source /tmp/libfns.sh && prepareServerPart2"

else
    echo 'unrecognized action: ${action}' 1>&2 && exit 1
fi
