#!/usr/bin/env bash

cd "$(dirname "$0")"

source libfns.sh

device=
lxcFs=
denyRestart=0
sbHost=
swapDevice=
skipIfExists=0
buildPackToInstall=

while getopts "b:d:f:hS:s:ne" OPTION; do
    case $OPTION in
        h)
            echo "usage: $0 -S [shipbuilder-host] -d [server-dedicated-device] -f [lxc-filesystem] ACTION" 1>&2
            echo '' 1>&2
            echo 'This is the ShipBuilder installer program.' 1>&2
            echo '' 1>&2
            echo '  ACTION                       Action to perform. Available actions are: install, list-devices'
            echo '  -S [shipbuilder-host]        ShipBuilder server user@hostname (flag can be omitted if auto-detected from env/SB_SSH_HOST)' 1>&2
            echo '  -d [server-dedicated-device] Device to format with btrfs or zfs filesystem and use to store lxc containers (e.g. /dev/xvdc)' 1>&2
            echo '  -f [lxc-filesystem]          LXC filesystem to use; "zfs" or "btrfs" (flag can be ommitted if auto-detected from env/LXC_FS)' 1>&2
            echo '  -s [swap-device]             Device to use for swap (optional)' 1>&2
            echo '  -n                           No reboot - deny system restart, even if one is required to complete installation' 1>&2
            echo '  -e                           Skip container preparation steps when the container already exists' 1>&2
            echo '' 1>&2
            echo '  -b [build-pack]              Install a single build-pack (and do not install or change anything else, LXC must already be installed)' 1>&2
            exit 1
            ;;
        d)
            device=$OPTARG
            ;;
        f)
            lxcFs=$OPTARG
            ;;
        n)
            denyRestart=1
            ;;
        S)
            sbHost=$OPTARG
            ;;
        s)
            swapDevice=$OPTARG
            ;;
        e)
            skipIfExists=1
            ;;
        b)
            buildPackToInstall=$OPTARG
            ;;
    esac
done

# Clear options from $n.
shift $(($OPTIND - 1))

action=$1

test -z "${sbHost}" && autoDetectServer
test -z "${lxcFs}" && autoDetectFilesystem
test -z "${zfsPool}" && autoDetectZfsPool

test -z "${sbHost}" && echo 'error: missing required parameter: -S [shipbuilder-host]' 1>&2 && exit 1
#test -z "${action}" && echo 'error: missing required parameter: action' 1>&2 && exit 1
if test -z "${action}"; then
    echo 'info: action defaulted to: install'
    action='install'
fi


verifySshAndSudoForHosts "${sbHost}"


getIpCommand="ifconfig | tr '\t' ' '| sed 's/ \{1,\}/ /g' | grep '^e[a-z]\+0[: ]' --after 8 | grep --only 'inet \(addr:\)\?[: ]*[^: ]\+' | tr ':' ' ' | sed 's/\(.*\) addr[: ]\{0,\}\(.*\)/\1 \2/' | sed 's/ \{1,\}/ /g' | cut -f2 -d' '"


function deployShipBuilder() {
    # Builds and installs shipbuilder deb.
    OLD_SHELLOPTS="$(set +o)"

    set -o errexit
    set -o pipefail
    set -o nounset

    sudo systemctl stop shipbuilder || :
    cd ..
    # TODO: Consider removing generate step, since it's included in `test'.
    envdir env bash -c 'make clean get generate test deb | tee /tmp/make.log'
    deb="$(tail -n1 /tmp/make.log | sed 's/^.*=>"\([^"]\+\)"}$/\1/')"
    test -n "${deb}" || (echo 'error: no deb artifact name detected, see /tmp/make.log for more information' 1>&2 && exit 1)
    sudo dpkg -i "dist/${deb}"
    sudo systemctl start shipbuilder

    $OLD_SHELLOPTS
}


function rsyncLibfns() {
    rsync -azve "ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no'" libfns.sh "${sbHost}:/tmp/"
    abortIfNonZero $? 'rsync libfns.sh failed'
}


if [ "${action}" = "list-devices" ]; then
    echo '----'
    ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' "${sbHost}" 'sudo find /dev/ -regex ".*\/\(\([hms]\|xv\)d\|disk\).*"'
    abortIfNonZero $? "retrieving storage devices from host ${sbHost}"
    exit 0

elif [ "${action}" = "build-deploy" ]; then
    deployShipBuilder

elif [ "${action}" = "install" ]; then
    if [ -n "${buildPackToInstall}" ]; then
        # Install a single build-pack.
        if ! [ -d "../build-packs/${buildPackToInstall}" ]; then
            knownBuildPacks="$(find ../build-packs -depth 1 -type d | cut -d'/' -f3 | tr '\n' ' ' | sed 's/ /, /g' | sed 's/, $//')"
            echo "error: unable to locate any build-pack named '${buildPackToInstall}', choices are: ${knownBuildPacks}" 1>&2
            exit 1
        fi
        deployShipBuilder
        rsyncLibfns
        ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' "${sbHost}" "source /tmp/libfns.sh && installSingleBuildPack ${buildPackToInstall} ${skipIfExists} ${lxcFs}"

    else
        # Perform a full ShipBuilder install.
        test -z "${device}" && echo 'error: missing required parameter: -d [device]' 1>&2 && exit 1
        test -z "${lxcFs}" && echo 'error: missing required parameter: -f [lxc-filesystem]' 1>&2 && exit 1

        installAccessForSshHost $sbHost
        abortIfNonZero $? 'installAccessForSshHost() failed'

        rsyncLibfns
        ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' "${sbHost}" "source /tmp/libfns.sh && prepareServerPart1 ${sbHost} ${device} ${lxcFs} ${zfsPool} ${swapDevice}"
        abortIfNonZero $? 'remote prepareServerPart1() invocation'

        deployShipBuilder

        ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' "${sbHost}" "source /tmp/libfns.sh && prepareServerPart2 ${skipIfExists} ${lxcFs}"
        abortIfNonZero $? 'remote prepareServerPart2() invocation'

        if test -z "${denyRestart}"; then
            echo 'info: checking if system restart is necessary'
            ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' "${sbHost}" "test -r '/tmp/SB_RESTART_REQUIRED' && test -n \"\$(cat /tmp/SB_RESTART_REQUIRED)\" && echo 'info: system restart required, restarting now' && sudo reboot || echo 'no system restart is necessary'"
            abortIfNonZero $? 'remote system restart check failed'
        else
            echo 'warn: a restart may be required on the shipbuilder server to complete installation, but the action was disabled by a flag' 1>&2
        fi
    fi

else
    echo "unrecognized action: ${action}" 1>&2 && exit 1
fi

