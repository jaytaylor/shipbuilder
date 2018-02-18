#!/usr/bin/env bash

source "$(dirname "$0")/libfns.sh"

checkUserPermissions

export lbHost=
export SB_SSH_HOST=
export certFile=

function main() {
    while getopts "H:S:c:h" OPTION; do
        case $OPTION in
            h)
                echo "usage: $0 -H [load-balancer-host] -S [shipbuilder-host] [ACTION]" 1>&2
                echo '' 1>&2
                echo 'This is the load-balancer installer.' 1>&2
                echo '' 1>&2
                echo '  ACTION                  Action to perform. Available actions are: install'
                echo '  -H [load-balancer-host] Load-balancer user@hostname' 1>&2
                echo '  -S [shipbuilder-host]   ShipBuilder server user@hostname (flag can be omitted if auto-detected from env/SB_SSH_HOST)' 1>&2
                echo '  -c [path-to-ssl-cert]   SSL certificate to use' 1>&2
                exit 1
                ;;
            H)
                export lbHost=${OPTARG}
                ;;
            S)
                export SB_SSH_HOST=${OPTARG}
                ;;
            c)
                export certFile=${OPTARG}
                ;;
        esac
    done

    # Clear options from $n.
    shift $((${OPTIND} - 1))

    action="${1:-install}"

    autoDetectVars

    # Validate required parameters.
    test -z "${SB_SSH_HOST:-}" && echo 'error: missing required parameter: -S [shipbuilder-host]' 1>&2 && exit 1
    test -z "${lbHost}" && echo 'error: missing required parameter: -H [load-balancer-host]' 1>&2 && exit 1
    #test -z "${action}" && echo 'error: missing required parameter: action' 1>&2 && exit 1

    test -n "${certFile}" && test ! -r "${certFile}" && echo "error: unable to read ssl certificate file; verify that it exists and user has permission to read it: ${certFile}" 1>&2 && exit 1
    test -z "${certFile}" && echo "warn: no ssl certificate file specified, ssl support will not be available (specify with '-c [path-to-ssl-cert]'" 1>&2

    verifySshAndSudoForHosts "${SB_SSH_HOST} ${lbHost}"

    if [ "${action}" = "install" ] ; then
        installAccessForSshHost "${lbHost}"

        rsync -azve "ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no'" "$(dirname "$0")/libfns.sh" ${lbHost}:/tmp/
        abortIfNonZero $? "rsyncing libfns.sh to host=${lbHost}"

        if [ -n "${certFile}" ] ; then
            rsync -azve "ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no'" "${certFile}" ${lbHost}:/tmp/
            abortIfNonZero $? "rsyncing certFile=${certFile} to host=${lbHost}"
        fi

        ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' ${lbHost} "source /tmp/libfns.sh && prepareLoadBalancer $(basename "${certFile}")"
        abortIfNonZero $? "load-balancer preparation for host=${lbHost}"

        ssh -o 'BatchMode=yes' -o 'StrictHostKeyChecking=no' ${lbHost} "sudo --non-interactive rm -rf /tmp/libfns.sh '/tmp/$(basename "${certFile}")'"
        abortIfNonZero $? "rsyncd files cleanup"

    else
        echo 'unrecognized action: ${action}' 1>&2 && exit 1
    fi
}

main $@

