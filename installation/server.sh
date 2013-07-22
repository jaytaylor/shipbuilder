#!/usr/bin/env bash

##
# @author Jay Taylor [@jtaylor]
#
# @date 2013-07-15
#

cd "$(dirname "$0")"

while getopts “d:hL:S:t” OPTION; do
    case $OPTION in
        d)
            DEVICE=$OPTARG
            ;;
        h)
            echo '  -S [host]         ShipBuilder server SSH address (e.g. ubuntu@my.sb)' 1>&2
            exit 1
            ;;
        S)
            sbHost=$OPTARG
            ;;
        t)
            DRY_RUN=1
            ;;
    esac
done

if [ -z "${sbHost}" ]; then
    echo 'error: missing required parameter: -S [HOST]' 1>&2
    exit 1
fi

if [ -z "${DEVICE}" ]; then
    echo 'error: missing required parameter: -d [DEVICE]' 1>&2
    exit 1
fi

if [ -n "${DRY_RUN}" ]; then
    exit 0
fi


function installGo() {
    if [ -z "$(which go)" ]; then
        echo 'info: Installing go 1.1 on the build-server'
        curl --location --silent https://launchpad.net/ubuntu/+archive/primary/+files/golang-src_1.1-1_amd64.deb > /tmp/golang-src_1.1-1_amd64.deb
        curl --location --silent https://launchpad.net/ubuntu/+archive/primary/+files/golang-go_1.1-1_amd64.deb > /tmp/golang-go_1.1-1_amd64.deb
        sudo dpkg -i /tmp/golang-src_1.1-1_amd64.deb /tmp/golang-go_1.1-1_amd64.deb
        mkdir ~/go 2>/dev/null
        echo 'info: Adding $GOPATH to ~/.bashrc'
        echo 'export GOPATH=$HOME/go' >> ~/.bashrc
    else
        echo 'info: Go already appears to be installed, not going to force it'
    fi
}

function setupBtrfs() {
    echo "info: Formatting device ${DEVICE} as BTRFS"
    bash setupBtrfs.sh -d $DEVICE
    rc=$?
    test $rc -ne 0 && echo "error: execution of setupBtrfs.sh exited with non-zero status ${rc}" && exit $rc
}

function gitLinkage() {
    echo 'info: Creating and linking /mnt/build/git -> /git'
    sudo mkdir -p /mnt/build/git 2>/dev/null
    sudo chown 777 /mnt/build/git
}

function buildEnv() {
    echo 'info: Add build servers hostname configuration'
    sudo mkdir /mnt/build/env 2>/dev/null
    echo "${sbHost}" | sudo tee /mnt/build/env/SB_SSH_HOST
}

function lxcInitBase() {
    echo 'info: Clear any pre-existing "base" container'
    sudo lxc-stop -n base --kill 2>/dev/null
    sudo lxc-destroy -n base 2>/dev/null
    
    echo 'info: Create the base LXC container'
    sudo lxc-create -n base -B btrfs -t ubuntu
    rc=$?
    test $rc -ne 0 && echo "error: lxc-create base exited with status ${rc}" 1>&2 && exit $rc
    
    echo 'info: Configuring base LXC container..'
    sudo lxc-start --daemon -n base
    rc=$?
    test $rc -ne 0 && echo "error: lxc-start base exited with status ${rc}" 1>&2 && exit $rc

    echo 'info: Getting container IP-address'
    allowedAttempts=60
    i=0
    while [ $i -lt $allowedAttempts ] && [ -z "${ip}" ]; do
        ip=$(sudo lxc-ls --fancy | grep '^base' | head -n1 | sed 's/[ \t]\+/ /g' | cut -d' ' -f3 | sed 's/[^0-9\.]*//g')
        i=$(($i+1))
        sleep 1
    done
    if [ -z "${ip}" ]; then
        echo "error: starting base container failed after ${allowedAttempts} attempts" 1>&2
        exit 1
    fi
}

function lxcConfigBase() {
    echo "info: Add our public-key to container's authorized_keys"
    sudo mkdir /mnt/build/lxc/base/rootfs/home/ubuntu/.ssh
    sudo cp ~/.ssh/id_rsa.pub /mnt/build/lxc/base/rootfs/home/ubuntu/.ssh/authorized_keys

    echo 'info: Adding the container "ubuntu" user to the sudoers list'
    echo 'ubuntu ALL=(ALL) NOPASSWD: ALL' | sudo tee -a /mnt/build/lxc/base/rootfs/etc/sudoers >/dev/null

    echo 'info: Updating apt repositories in container'
    ssh -o 'StrictHostKeyChecking no' -o 'BatchMode yes' ubuntu@$ip sudo apt-get update
    rc=$?
    test $rc -ne 0 && echo "error: container apt-get update exited with status ${rc}" 1>&2 && exit $rc

    packages='daemontools git-core curl python-pip python-virtualenv python-dev libpq-dev libxml2 libxml2-dev libxslt1.1 libxslt1-dev python-libxml2 libcurl4-gnutls-dev libmemcached-dev zlib1g-dev'
    echo "info: Installing packages to base containers: ${packages}"
    ssh -o 'StrictHostKeyChecking no' -o 'BatchMode yes' ubuntu@$ip "sudo apt-get install -y ${packages}"
    rc=$?
    test $rc -ne 0 && echo "error: container apt-get install ${packages} exited with status ${rc}" 1>&2 && exit $rc

    echo 'info: stopping base container'
    sudo lxc-stop -k -n base
    rc=$?
    test $rc -ne 0 && echo "error: lxc-stop base exited with status ${rc}" 1>&2 && exit $rc
}

function rsyslogLoggingListeners() {
    echo 'info: Enable rsyslog listeners'
    echo '# UDP syslog reception.
    $ModLoad imudp
    $UDPServerAddress 0.0.0.0
    $UDPServerRun 514

    # TCP syslog reception.
    $ModLoad imtcp
    $InputTCPServerRun 10514' | sudo tee /etc/rsyslog.d/49-haproxy.conf
    echo 'info: Restart rsyslog'
    sudo service rsyslog restart
    if [ -e /etc/rsyslog.d/haproxy.conf ]; then
        echo 'info: detected existing rsyslog haproxy configuration, will disable it'
        sudo mv /etc/rsyslog.d/haproxy.conf /etc/rsyslog.d-haproxy.conf.disabled
    fi
}

source installLxc.sh

installGo
installLxc
setupBtrfs
gitLinkage
buildEnv
lxcInitBase
lxcConfigBase
rsyslogLoggingListeners

exit 0

