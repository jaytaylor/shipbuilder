#!/usr/bin/env bash

cd "$(dirname "$0")"

source libfns.sh

export device=
export SB_LXC_FS=
export nodeHost=
export denyRestart=
export SB_SSH_HOST=
export swapDevice=

function main() {
    local OPTION
    local OPTIND
    local action

    while getopts "d:f:H:hnS:s:" OPTION; do
        case ${OPTION} in
            h)
                echo "usage: $0 -H [node-host] -S [shipbuilder-host] -d [device] -f [lxc-filesystem] [ACTION]" 1>&2
                echo '' 1>&2
                echo 'This is the node installer.' 1>&2
                echo '' 1>&2
                echo 'IMPORTANT: Do not run on the node where `shipbuilder` is running.' 1>&2
                echo '' 1>&2
                echo '  ACTION                Action to perform. Available actions are: list-devices, install'
                echo '  -S [shipbuilder-host] ShipBuilder server user@hostname (flag can be omitted if auto-detected from env/SB_SSH_HOST)' 1>&2
                echo '  -H [node-host]        Node user@hostname' 1>&2
                echo '  -d [device]           Device to install filesystem on' 1>&2
                echo '  -f [lxc-filesystem]   LXC filesystem to use; "zfs" or "btrfs" (flag can be ommitted if auto-detected from env/LXC_FS)' 1>&2
                echo '  -n                    No reboot - deny system restart, even if one is required to complete installation' 1>&2
                echo '  -s [swap-device]      Device to use for swap (optional)' 1>&2
                exit 1
                ;;
            d)
                export device=${OPTARG}
                ;;
            f)
                export SB_LXC_FS=${OPTARG}
                ;;
            H)
                export nodeHost=${OPTARG}
                ;;
            n)
                export denyRestart=1
                ;;
            S)
                export SB_SSH_HOST=${OPTARG}
                ;;
            s)
                export swapDevice=${OPTARG}
                ;;
        esac
    done

    # Clear options from $n.
    shift $((${OPTIND} - 1))

    action=${1:-install}

    test -z "${SB_SSH_HOST:-}" && autoDetectServer
    test -z "${SB_LXC_FS:-}" && autoDetectFilesystem
    test -z "${SB_ZFS_POOL:-}" && autoDetectZfsPool

    # Validate required parameters.
    test -z "${SB_SSH_HOST:-}" && echo 'error: missing required parameter: -S [shipbuilder-host]' 1>&2 && exit 1
    test -z "${nodeHost:-}" && echo 'error: missing required parameter: -H [node-host]' 1>&2 && exit 1

    if test -z "${action}"; then
        echo 'info: action defaulting to: install'
        action='install'
    fi


    verifySshAndSudoForHosts "${SB_SSH_HOST} ${nodeHost}"


    if [ "${action}" = "list-devices" ]; then
        echo '----'
        ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' "${nodeHost}" 'sudo find /dev/ -regex ".*\/\(\([hms]\|xv\)d\|disk\).*"'
        abortIfNonZero $? "retrieving storage devices from host ${SB_SSH_HOST}"
        exit 0

    elif [ "${action}" = "install" ]; then
        test -z "${device}" && echo 'error: missing required parameter: -d [device]' 1>&2 && exit 1
        test -z "${SB_LXC_FS}" && echo 'error: missing required parameter: -f [lxc-filesystem]' 1>&2 && exit 1

        installAccessForSshHost "${nodeHost}"

        rsync -azve "ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no'" libfns.sh "${nodeHost}:/tmp/"
        abortIfNonZero $? 'rsync libfns.sh failed'

        ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' "${nodeHost}" "source /tmp/libfns.sh && prepareNode ${device} ${SB_LXC_FS} ${SB_ZFS_POOL} ${swapDevice}"
        abortIfNonZero $? 'remote prepareNode() invocation'

        ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' "${nodeHost}" 'sudo lxc remote add sb-server ${SB_SSH_HOST} --accept-certificate --public && sudo cp -a ${USER}/.config /root/'
        abortIfNonZero $? 'adding sb-server lxc image server to slave node'

        ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' "${nodeHost}" sudo bash -c 'set -o errexit ; set -o pipefail ; set -x ; sed -i "s/net.ipv4.conf.all.route_localnet *=.*//d" /etc/sysctl.conf && sysctl -w $(echo "net.ipv4.conf.all.route_localnet=1" | sudo tee -a /etc/sysctl.conf)'
        abortIfNonZero $? 'setting sysctl -w net.ipv4.conf.all.route_localnet=1 on slave node'

        if test -z "${denyRestart}"; then
            echo 'info: checking if system restart is necessary'
            ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' "${nodeHost}" "test -r '/tmp/SB_RESTART_REQUIRED' && test -n \"\$(cat /tmp/SB_RESTART_REQUIRED)\" && echo 'info: system restart required, restarting now' && sudo reboot || echo 'no system restart is necessary'"
            abortIfNonZero $? 'remote system restart check failed'
        else
            echo 'warn: a restart may be required on the node to complete installation, but the action was disabled by a flag' 1>&2
        fi

    else
        echo 'unrecognized action: ${action}' 1>&2 && exit 1
    fi
}

main $@