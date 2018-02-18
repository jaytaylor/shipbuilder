#!/usr/bin/env bash

source "$(dirname "$0")/libfns.sh"

checkUserPermissions

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

    autoDetectVars

    # Validate required parameters.
    test -z "${SB_SSH_HOST:-}" && echo 'error: missing required parameter: -S [shipbuilder-host]' 1>&2 && exit 1
    test -z "${nodeHost:-}" && echo 'error: missing required parameter: -H [node-host]' 1>&2 && exit 1

    if test -z "${action}"; then
        echo 'info: action defaulting to: install'
        action='install'
    fi

    verifySshAndSudoForHosts "${SB_SSH_HOST} ${nodeHost}"

    if [ "${action}" = "list-devices" ] ; then
        echo '----'
        ${SB_SSH} "${nodeHost}" "${SB_SUDO}"' find /dev/ -regex ".*\/\(\([hms]\|xv\)d\|disk\).*"'
        abortIfNonZero $? "retrieving storage devices from host ${SB_SSH_HOST}"
        exit 0

    elif [ "${action}" = "install" ] ; then
        test -z "${device}" && echo 'error: missing required parameter: -d [device]' 1>&2 && exit 1
        test -z "${SB_LXC_FS}" && echo 'error: missing required parameter: -f [lxc-filesystem]' 1>&2 && exit 1

        installAccessForSshHost "${nodeHost}"

        rsync -azve "${SB_SSH}" "$(dirname "$0")/libfns.sh" "${nodeHost}:/tmp/"
        abortIfNonZero $? 'rsync libfns.sh to nodeHost=${nodeHost}'


        if [ -d "${HOME}/.config" ] ; then
            rsync -azve "${SB_SSH}" "${HOME}/.config" "${nodeHost}:~/"
            abortIfNonZero $? "rsync LXD HTTPS client key and cert from ~/.config over to host=${nodeHost}"
        else
            echo 'info: no ${HOME}/.config found'
        fi

        ${SB_SSH} "${nodeHost}" "source /tmp/libfns.sh && $(dumpAutoDetectedVars) prepareNode ${device} ${SB_LXC_FS} ${SB_ZFS_POOL} ${swapDevice}"
        abortIfNonZero $? 'remote prepareNode() invocation'
        set +x

        ${SB_SSH} "${nodeHost}" "${SB_SUDO} lxc remote add --accept-certificate --public sb-server ${SB_SSH_HOST} && ${SB_SUDO}"' cp -a ${USER}/.config /root/'
        abortIfNonZero $? 'adding sb-server lxc remote image server to slave node'

        ${SB_SSH} "${nodeHost}" -- /bin/bash -c 'set -o errexit && set -o pipefail && sudo --non-interactive sed -i "/^[ \t]*net\.ipv4\.conf\.all\.route_localnet *=.*/d" /etc/sysctl.conf && sudo --non-interactive sysctl -w $(echo "net.ipv4.conf.all.route_localnet=1" | sudo --non-interactive tee -a /etc/sysctl.conf)'
        abortIfNonZero $? 'setting sysctl -w net.ipv4.conf.all.route_localnet=1 on slave node'

        if test -z "${denyRestart}"; then
            echo 'info: checking if system restart is necessary'
            ${SB_SSH} "${nodeHost}" "test -r '/tmp/SB_RESTART_REQUIRED' && test -n \"\$(cat /tmp/SB_RESTART_REQUIRED)\" && echo 'info: system restart required, restarting now' && ${SB_SUDO} reboot || echo 'no system restart is necessary'"
            abortIfNonZero $? 'remote system restart check failed'
        else
            echo 'warn: a restart may be required on the node to complete installation, but the action was disabled by a flag' 1>&2
        fi

    else
        echo 'unrecognized action: ${action}' 1>&2 && exit 1
    fi
}

main $@

