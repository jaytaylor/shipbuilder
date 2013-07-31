#!/usr/bin/env bash

##
# @author Jay Taylor [@jtaylor]
#
# @date 2013-07-15
#

RESOURCES='server.sh setupBtrfs.sh loadBalancer.sh'

while getopts “c:hL:N:n:S:s:tz” OPTION; do
    case $OPTION in
        c)
            certFile=$OPTARG
            ;;
        h)
            echo "usage: $0 -L [load-balancer-host] -c [ssl-cert] -N [node-hosts(comma-delimited)] -n  -S [shipbuilder-host] -d [server-btrfs-device]" 1>&2
            echo '' 1>&2
            echo 'This is the ShipBuilder installer program.' 1>&2
            echo '' 1>&2
#            for resource in $RESOURCES; do 
#                bash $resource $*
#            done
            echo '  -c [ssl-cert]                  SSL certificate to use with HAProxy' 1>&2
            echo '  -L [host]                      Load-balancer server SSH address (e.g. ubuntu@my.lb)' 1>&2
            echo '  -N [host,host,..]              Comma-delimited list of Container Node SSH addresses (e.g. ubuntu@my.node1,ubuntu@my.node2)' 1>&2
            echo '  -n [node-btrfs-device]         Container Node BTRFS device (e.g. /dev/xvdb)' 1>&2
            echo '  -S [host]                      ShipBuilder Server SSH address (e.g. ubuntu@my.sb)' 1>&2
            echo '  -s [sb-server-btrfs-device]    ShipBuilder Server BTRFS device (e.g. /dev/xvdc)' 1>&2
            echo '  -t                             Test for valid parameters and then quit (dry-run only)' 1>&2
            echo '  -z                             List all remote filesystem devices and exit' 1>&2
            exit 1
            ;;
        L)
            lbHost=$OPTARG
            ;;
        N)
            nodeHosts=$OPTARG
            ;;
        n)
            nodeDevice=$OPTARG
            ;;
        S)
            sbHost=$OPTARG
            ;;
        s)
            sbDevice=$OPTARG
            ;;
        t)
            echo 'info: test mode enabled, dry-run only'
            DRY_RUN=1
            ;;
        z)
            LIST_DEVICES_ONLY=1
            ;;
    esac
done

# Validate parameters.
if [ -n "${lbHost}" ] && [ -z "${certFile}" ]; then
    echo 'warn: missing ssl-cert paramter: -c [ssl-cert], no SSL support will be available' 1>&2
    #echo 'error: missing required paramter: -c [ssl-cert]' 1>&2
    #exit 1
fi

if [ -n "${lbHost}" ] && ! [ -r "${certFile}" ]; then
    echo "warn: unable to read ssl certificate file (may not exist): ${certFile}" 1>&2
    #echo "error: unable to read ssl certificate file (may not exist): ${certFile}" 1>&2
    #exit 1
fi


sshHosts="${sbHost} ${lbHost} $(echo "${nodeHosts}" | tr ',' ' ')"

source libfns.sh
verifySshAndSudoForHosts "${sshHosts}" 

if [ -n "${LIST_DEVICES_ONLY}" ]; then
    echo 'info: listing all remote devices'
    for sshHost in $(echo "${sshHosts}"); do
        echo "info:     ${sshHost}"
        devices=$(ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' -q $sshHost 'sudo find /dev/ -regex ".*\/\([hms\|]xv\)d.*"')
        rc=$?
        test $rc -ne 0 && echo "error: failed to retrieve storage devices from host ${sshHost}, command exited with status ${rc}" 1>&2 && exit $rc
        for device in $(echo "${devices}"); do
            echo "info:         ${device}"
        done
        echo ''
    done
    exit 0
fi

if [ -n "${sbHost}" ] && [ -z "${sbDevice}" ]; then
    echo 'error: missing required paramter: -s [sb-server-btrfs-device]' 1>&2
    exit 1
fi

if [ -n "${nodeHosts}" ] && [ -z "${nodeDevice}" ]; then
    echo 'error: missing required paramter: -n [node-btrfs-device]' 1>&2
    exit 1
fi

if [ -n "${DRY_RUN}" ]; then
    echo 'info: validating parameters..' 1>&2
    for resource in $RESOURCES; do
        bash $resource $*
        rc=$?
        if [ $rc -ne 0 ]; then
            echo "caught error in ${resource} (return code was ${rc}), bailing out." 1>&2
            exit $rc
        fi
    done
    exit 0
fi


getIpCommand="ifconfig | tr '\t' ' '| sed 's/ \{1,\}/ /g' | grep '^e[a-z]\+0[: ]' --after 8 | grep --only 'inet \(addr:\)\?[: ]*[^: ]\+' | tr ':' ' ' | sed 's/\(.*\) addr[: ]\{0,\}\(.*\)/\1 \2/' | sed 's/ \{1,\}/ /g' | cut -f2 -d' '"

function installAccessForSshHost() {
    # @precondition Variable $sshKeysCommand must be initialized and not empty.
    # @param $1 SSH connection string (e.g. user@host)
    test -z "${sshKeysCommand}" && echo "error: installAccessForSshHost(\$sshHost=${sshHost}) invoked with empty \$sshKeysCommand" 1>&2 && exit 1
    local sshHost="$1"
    echo "info: Setting up remote access from build-server to host: ${sshHost}"
    echo "info: Installing build-server unprivileged and root users' public-keys on host: ${sshHost}"
    ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $sshHost "${sshKeysCommand}"
}

function configureSshAccess() {
    sbMarker='.ssh/.shipbuilder-install-marker'
    echo 'info: Setting up ssh access on build-server'
    ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $sbHost '/bin/bash -c "'"
    echo 'remote: info: Setting up pub/private SSH keys so that root can SSH in as both unprivileged and root users'
    if ! test -e ~/${sbMarker}; then
        echo 'remote: info: For unprivileged user:'
        if ! test -r ~/.ssh/id_rsa || ! test -r ~/.ssh/id_rsa.pub; then
            echo 'remote: info: Generating new priv/pub key for unprivileged user'
            rm -f ~/.ssh/id_*
            ssh-keygen -f ~/.ssh/id_rsa -t rsa -N ''
        fi
        cat ~/.ssh/id_rsa.pub | tee -a ~/.ssh/authorized_keys >/dev/null
        chmod 600 ~/.ssh/authorized_keys
        echo 'shipbuilder installed on '$(date)', do not remove this file' > ~/${sbMarker}
    else
        echo 'remote: info: Skipping for unprivileged user because this has already run here (~/${sbMarker} exists)'
    fi

    if ! sudo test -e /root/${sbMarker}; then
        echo 'remote: info: For root user:'
        if ! sudo test -e /root/.ssh/id_rsa || ! sudo test -e /root/.ssh/id_rsa.pub; then
            echo 'remote: info: Generating new priv/pub key for root'
            sudo /bin/bash -c 'rm -f /root/.ssh/id_*'
            sudo -E ssh-keygen -f /root/.ssh/id_rsa -t rsa -N ''
        fi
        sudo cat /root/.ssh/id_rsa.pub | sudo tee -a /root/.ssh/authorized_keys >/dev/null
        sudo chmod 600 /root/.ssh/authorized_keys
        echo 'remote: info: Adding root user to unprivileged users authorized_keys'
        sudo cat /root/.ssh/id_rsa.pub | tee -a ~/.ssh/authorized_keys >/dev/null
        echo 'shipbuilder installed on '$(date)', do not remove this file' | sudo tee /root/${sbMarker} >/dev/null
    else
        echo 'remote: info: Skipping for root user because this has already run here (/root/${sbMarker} exists)'
    fi"'"'

    #sbInternalIp=$(ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $sbHost "${getIpCommand} | tail -n1")
    #echo "info: Found build-server IP-address: ${sbInternalIp}"

    echo "info: getting unprivileged user and root public-keys from ${sbHost}"
    pubKeys=$(ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $sbHost 'cat ~/.ssh/id_rsa.pub; echo "."; sudo cat /root/.ssh/id_rsa.pub')
    unprivilegedPubKey=$(echo "${pubKeys}" | grep --before 100 '^\.$' | grep -v '^\.$')
    rootPubKey=$(echo "${pubKeys}" | grep --after 100 '^\.$' | grep -v '^\.$')
    if [ -z "${unprivilegedPubKey}" ]; then
        echo 'error: failed to obtain build-server public-key for unprivileged user' 1>&2
        exit 1
    fi
    if [ -z "${rootPubKey}" ]; then
        echo 'error: failed to obtain build-server public-key for root user' 1>&2
        exit 1
    fi

    sshKeysCommand='/bin/bash -c "'"
    if ! test -e ~/${sbMarker}; then
        echo -e '${unprivilegedPubKey}\n${rootPubKey}' >> .ssh/authorized_keys
        chmod 600 .ssh/authorized_keys
        echo 'shipbuilder installed on '$(date)', do not remove this file' > ~/${sbMarker}
    else
        echo 'remote: info: Skipping for root user because this has already run here (/root/${sbMarker} exists)'
    fi
    if ! sudo test -e /root/${sbMarker}; then
        echo -e '${unprivilegedPubKey}\n${rootPubKey}' | sudo tee -a /root/.ssh/authorized_keys >/dev/null
        sudo chmod 600 /root/.ssh/authorized_keys
        echo 'shipbuilder installed on '$(date)', do not remove this file' | sudo tee /root/${sbMarker} >/dev/null
    else
        echo 'remote: info: Skipping for unprivileged user because this has already run here (~/${sbMarker} exists)'
    fi
    "'"'
    echo "info: sshKeysCommand is :::${sshKeysCommand}:::"

    echo 'info: Installing build-server SSH keys on all remote hosts'

    if [ -n "${lbHost}" ]; then
        if [ "${lbHost}" = "${sbHost}" ]; then
            echo 'info: build-server is the same as the load-balancer, skipping ssh key modification step'
            #lbInternalIp=$sbInternalIp
        else
            installAccessForSshHost $lbHost
            #lbInternalIp=$(ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $lbHost "${getIpCommand} | tail -n1")
            #echo "info: Found load-balancer IP-address: ${lbInternalIp}"
        fi
    else
        echo 'warn: load-balancer ssh keys: no host was specified'
    fi

    if [ -n "${nodeHosts}" ]; then
        echo "info: adding 'ubuntu' and 'root' public-keys to nodes.."
        declare -a nodeInternalIps=()
        for nodeHost in $(echo "${nodeHosts}" | tr ',' ' '); do
            if [ "${nodeHost}" = "${sbHost}" ] || [ "${nodeHost}" = "${lbHost}" ]; then
                echo 'info: build-server is the same as the node, skipping ssh key modification step'
                #internalIp=$sbInternalIp
            else
                installAccessForSshHost $nodeHost
                #internalIp=$(ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $nodeHost "${getIpCommand} | tail -n1")
                #echo "info: Found node IP-address: ${internalIp}"
            fi
            #nodeInternalIps=("${nodeInternalIps[@]}" "${internalIp}")
        done
    else
        echo 'warn: nodes ssh keys: no node hosts were specified'
    fi
}

function installServer() {
    echo "info: Installing build-server on host: ${sbHost}"
    rsync -azve "ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no'" server.sh setupBtrfs.sh libfns.sh $sbHost:/tmp/
    ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $sbHost "/bin/bash /tmp/server.sh -S ${sbHost} -d ${sbDevice}"
    rc=$?
    test $rc -ne 0 && echo "error: remote execution of server.sh exited with non-zero status ${rc}" 1>&2 && exit $rc
    ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $sbHost "rm -f /tmp/server.sh /tmp/setupBtrfs.sh /tmp/libfns.sh"
}

function setupLoadBalancer() {
    if [ -n "${lbHost}" ]; then
        echo 'info: Setting up load-balancer'
        rsync -azve "ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no'" $certFile loadBalancer.sh $lbHost:/tmp/
        certFileRemotePath="/tmp/$(echo $certFile | sed 's/^.*\/\(.*\)$/\1/')"
        ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $lbHost "/bin/bash /tmp/loadBalancer.sh -c ${certFileRemotePath}"
        rc=$?
        test $rc -ne 0 && echo "error: remote execution of loadBalancer.sh exited with non-zero status ${rc}" 1>&2 && exit $rc
        ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $lbHost "rm -f /tmp/loadBalancer.sh"
    else
        echo 'warn: skipping load-balancer install, no host provided'
    fi
}

function setupNodes() {
    if [ -n "${nodeHosts}" ]; then
        echo 'info: Setting up nodes'
        for nodeHost in $(echo "${nodeHosts}" | tr ',' ' '); do
            if [ "${sbHost}" = "${nodeHost}" ]; then
                echo "info: build-server is the same as the node ${nodeHost}, nothing required"
            else
                echo "info: Setting up node: ${nodeHost}"
                rsync -azve "ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no'" setupBtrfs.sh libfns.sh $nodeHost:/tmp/
                ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $nodeHost "/bin/bash /tmp/setupBtrfs.sh -d ${nodeDevice}"
                rc=$?
                test $rc -ne 0 && echo "error: remote execution of setupBtrfs.sh exited with non-zero status ${rc}" 1>&2 && exit $rc
                ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $nodeHost "rm -f /tmp/setupBtrfs.sh /tmp/libfns.sh"
            fi
        done
    else
        echo 'warn: skipping nodes install, no host(s) provided'
    fi
}


configureSshAccess
setupLoadBalancer
setupNodes
installServer

