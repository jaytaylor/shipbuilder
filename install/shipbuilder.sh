#!/usr/bin/env bash

source "$(dirname "$0")/libfns.sh"

checkUserPermissions

export device=
export SB_LXC_FS=
export denyRestart=0
export SB_SSH_HOST=
export swapDevice=
export skipIfExists=0
export buildPackToInstall=

function deployShipBuilder() {
    local setstates
    local deb

    # Store state of xtrace option.
    setstates="$(set +o)"

    # Bind state restoration to RETURN trap.
    # trap "${__restorestates_trap}" RETURN
    trap 'IFS=$'"'"'\n'"'"' ; for ss in $(echo -e "${setstates}" | sort --reverse -k3) ; do eval ${ss} 1>/dev/null 2>/dev/null ; done ; trap - RETURN' RETURN

    # NB: Ignore possible unhappy exit status codes since shipbuilder service
    # may not yet exist.
    sudo --non-interactive systemctl stop shipbuilder || :

    # set -o errexit
    set -o pipefail

    cd "$(dirname "$0")/.."

    test ! -f /etc/profile.d/Z99-go.sh || source /etc/profile.d/Z99-go.sh

    command -v go >/dev/null || installGo
    abortIfNonZero $? "command: 'installGo'"

    command -v envdir >/dev/null || ( sudo --non-interactive apt update && sudo --non-interactive apt install -y daemontools )
    abortIfNonZero $? "command: 'apt install -y daemontools'"

    command -v make >/dev/null || ( sudo --non-interactive apt update && sudo --non-interactive apt install -y build-essential )
    abortIfNonZero $? "command: 'apt install -y build-essential'"

    sudo --non-interactive mkdir -p /etc/shipbuilder
    abortIfNonZero $? "command: 'mkdir -p /etc/shipbuilder'"

    # TODO: Consider removing generate step, since it's included in `test'.
    envdir env bash -c 'make clean get generate deb | tee /tmp/sb-build.log'
    deb="$(tail -n 1 /tmp/sb-build.log | sed 's/^.*=>"\([^"]\+\)"}$/\1/')"
    if [ -z "${deb}" ] ; then
        echo 'error: no deb artifact name detected, see /tmp/sb-build.log for more information' 1>&2
        return 1
    fi

    sudo --non-interactive dpkg -i "dist/${deb}"
    abortIfNonZero $? "command: 'dpkg -i dist/${deb}'"

    sudo --non-interactive systemctl daemon-reload
    abortIfNonZero $? "command: 'systemctl daemon-reload'"

    sudo --non-interactive systemctl start shipbuilder
    abortIfNonZero $? "command: 'systemctl start shipbuilder'"

    cd -
}

function rsyncLibfns() {
    pwd
    rsync -azve "ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no'" "$(dirname "$0")/libfns.sh" "$(dirname "$0")/lxd.yaml" "${SB_SSH_HOST}:/tmp/"
    abortIfNonZero $? 'rsync libfns.sh failed'
}

function main() {
    local OPTION
    local OPTIND
    local action
    local knownBuildPacks

    while getopts "b:d:f:hS:s:ne" OPTION; do
        case ${OPTION} in
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
                export device=${OPTARG}
                ;;
            f)
                export SB_LXC_FS=${OPTARG}
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
            e)
                export skipIfExists=1
                ;;
            b)
                export buildPackToInstall=${OPTARG}
                ;;
        esac
    done

    # Clear options from $n.
    shift $((${OPTIND} - 1))

    action=${1:-}

    autoDetectVars

    test -z "${SB_SSH_HOST:-}" && echo 'error: missing required parameter: -S [shipbuilder-host]' 1>&2 && exit 1

    if test -z "${action}"; then
        echo 'info: action defaulting to: install'
        action='install'
    fi

    verifySshAndSudoForHosts "${SB_SSH_HOST}"

    getIpCommand="ip addr | grep '^[0-9]\+: e[a-z]\+[0-9][: ]' --after 8 | grep --only-matching ' inet [^ \/]\+' | awk '{print \$2}'"

    if [ "${action}" = 'list-devices' ] ; then
        echo '----'
        ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' "${SB_SSH_HOST}" 'sudo find /dev/ -regex ".*\/\(\([hms]\|xv\)d\|disk\).*"'
        abortIfNonZero $? "retrieving storage devices from host ${SB_SSH_HOST}"
        exit 0

    elif [ "${action}" = 'deploy' ] || [ "${action}" = 'build-deploy' ] ; then
        deployShipBuilder
        abortIfNonZero $? 'building and deploying shipbuilder binary'

    elif [ "${action}" = 'buildpacks' ] || [ "${action}" = 'build-packs' ] ; then
        rsyncLibfns

        ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' "${SB_SSH_HOST}" "source /tmp/libfns.sh && prepareServerPart2 ${skipIfExists} ${SB_LXC_FS}"
        abortIfNonZero $? 'buildpacks: remote prepareServerPart2() invocation'

    elif [ "${action}" = 'install' ] ; then
        if [ -n "${buildPackToInstall}" ] ; then
            deployShipBuilder
            abortIfNonZero $? 'building and deploying shipbuilder binary'

            # Install a single build-pack.
            if ! [ -d "$(dirname "$0")/../build-packs/${buildPackToInstall}" ] ; then
                knownBuildPacks="$( \
                    find "$(dirname "$0")/../build-packs" -maxdepth 1 -type d \
                    | cut -d'/' -f5 \
                    | tr '\n' ' ' \
                    | grep -v '^$' \
                    | sed 's/ /, /g' \
                    | sed 's/, $//' \
                )"
                echo "error: unable to locate any build-pack named '${buildPackToInstall}', choices are: ${knownBuildPacks}" 1>&2
                exit 1
            fi

            rsyncLibfns
            ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' "${SB_SSH_HOST}" "source /tmp/libfns.sh && installSingleBuildPack ${buildPackToInstall} ${skipIfExists} ${SB_LXC_FS}"
            abortIfNonZero $? 'remote installSingleBuildPack() invocation'

        else
            # Perform a full ShipBuilder install.
            test -z "${device}" && echo 'error: missing required parameter: -d [device]' 1>&2 && exit 1
            test -z "${SB_LXC_FS}" && echo 'error: missing required parameter: -f [lxc-filesystem]' 1>&2 && exit 1

            installAccessForSshHost "${SB_SSH_HOST}"
            abortIfNonZero $? 'installAccessForSshHost() failed'

            deployShipBuilder

            rsyncLibfns
            ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' "${SB_SSH_HOST}" "source /tmp/libfns.sh && $(dumpAutoDetectedVars) prepareServerPart1 ${SB_SSH_HOST} ${device} ${SB_LXC_FS} ${SB_ZFS_POOL} ${swapDevice}"
            abortIfNonZero $? 'remote prepareServerPart1() invocation'

            ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' "${SB_SSH_HOST}" "set -o errexit ; sudo lxc config set core.https_address '[::]:8443'"
            abortIfNonZero $? 'activating lxc image server'

            # # Enable the LXC image server to bind on 0.0.0.0.
            # ${SB_SUDO} lxc config set core.https_address '[::]:8443'
            # abortIfNonZero $? "enabling the LXC image server to bind on 0.0.0.0"

            ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' "${SB_SSH_HOST}" "source /tmp/libfns.sh && $(dumpAutoDetectedVars) prepareServerPart2 ${skipIfExists} ${SB_LXC_FS}"
            abortIfNonZero $? 'remote prepareServerPart2() invocation'

            if test -z "${denyRestart}"; then
                echo 'info: checking if system restart is necessary'
                ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' "${SB_SSH_HOST}" "test -r '/tmp/SB_RESTART_REQUIRED' && test -n \"\$(cat /tmp/SB_RESTART_REQUIRED)\" && echo 'info: system restart required, restarting now' && sudo reboot || echo 'no system restart is necessary'"
                abortIfNonZero $? 'remote system restart check failed'
            else
                echo 'warn: a restart may be required on the shipbuilder server to complete installation, but the action was disabled by a flag' 1>&2
            fi
        fi

    else
        echo "unrecognized action: ${action}" 1>&2 && exit 1
    fi
}

main $@

