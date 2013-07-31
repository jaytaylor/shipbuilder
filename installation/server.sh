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
    abortIfNonZero $? "execution of setupBtrfs.sh"
}

function gitLinkage() {
    echo 'info: Creating and linking /mnt/build/git -> /git'
    sudo mkdir -p /mnt/build/git 2>/dev/null
    sudo chown 777 /mnt/build/git
    sudo ln -s /mnt/build/git /git
}

function buildEnv() {
    echo 'info: Add build servers hostname configuration'
    sudo mkdir /mnt/build/env 2>/dev/null
    echo "${sbHost}" | sudo tee /mnt/build/env/SB_SSH_HOST
}

function getContainerIp() {
    local container="$1"
    local allowedAttempts=60
    local i=0
    echo "info: Getting container IP-address for name '${container}'"
    while [ $i -lt $allowedAttempts ]; do
        maybeIp=$(sudo lxc-ls --fancy | grep "^${container}[ \t]\+" | head -n1 | sed 's/[ \t]\+/ /g' | cut -d' ' -f3 | sed 's/[^0-9\.]*//g')
        # Verify that after a few seconds the ip hasn't changed.
        if [ -n "${maybeIp}" ]; then
            sleep 5
            ip=$(sudo lxc-ls --fancy | grep "^${container}[ \t]\+" | head -n1 | sed 's/[ \t]\+/ /g' | cut -d' ' -f3 | sed 's/[^0-9\.]*//g')
            if [ "${ip}" = "${maybeIp}" ]; then
                echo "info: ip-address verified, value=${ip}"
                break
            else
                echo "warn: ip-address not stable try1=${maybeIp} try2=${ip}"
            fi
            echo "info: found an ip=${ip}"
        fi
        i=$(($i+1))
        sleep 1
    done
    if [ -n "${ip}" ]; then
        echo "info: found ip=${ip} for ${container} container"
    else
        echo "error: obtaining ip-address for container '${container}' failed after ${allowedAttempts} attempts" 1>&2
        exit 1
    fi
}

function lxcInitBase() {
    echo 'info: Clear any pre-existing "base" container'
    sudo lxc-stop -k -n base 2>/dev/null
    sudo lxc-destroy -n base 2>/dev/null
    
    echo 'info: Create the base LXC container'
    sudo lxc-create -n base -B btrfs -t ubuntu
    abortIfNonZero $? "lxc-create base"
    
    echo 'info: Configuring base LXC container..'
    sudo lxc-start --daemon -n base
    abortIfNonZero $? "lxc-start base"

    getContainerIp base
}

function lxcConfigBase() {
    echo "info: Add our public-key to container's authorized_keys"
    sudo mkdir /mnt/build/lxc/base/rootfs/home/ubuntu/.ssh
    sudo cp ~/.ssh/id_rsa.pub /mnt/build/lxc/base/rootfs/home/ubuntu/.ssh/authorized_keys

    echo 'info: Adding the container "ubuntu" user to the sudoers list'
    echo 'ubuntu ALL=(ALL) NOPASSWD: ALL' | sudo tee -a /mnt/build/lxc/base/rootfs/etc/sudoers >/dev/null

    echo 'info: Updating apt repositories in container'
    ssh -o 'StrictHostKeyChecking no' -o 'BatchMode yes' ubuntu@$ip sudo apt-get update
    abortIfNonZero $? "container apt-get update"

    packages='daemontools git-core curl unzip'
    echo "info: Installing packages to base container: ${packages}"
    ssh -o 'StrictHostKeyChecking no' -o 'BatchMode yes' ubuntu@$ip "sudo apt-get install -y ${packages}"
    abortIfNonZero $? "container apt-get install ${packages}"

    echo 'info: stopping base container'
    sudo lxc-stop -k -n base
    abortIfNonZero $? "lxc-stop base"
}

function lxcConfigBuildPack() {
    # @param $1 base-container suffix (e.g. 'python').
    # @param $2 list of packages to install.
    # @param $3 customCommands command to evaluate over SSH.
    local container="base-$1"
    local packages="$2"
    local customCommands="$3"
    echo "info: Creating build-pack ${container} container"
    sudo lxc-clone -s -B btrfs -o base -n $container
    sudo lxc-start -d -n $container
    getContainerIp $container

    echo "info: Installing packages to ${container} container: ${packages}"
    ssh -o 'StrictHostKeyChecking no' -o 'BatchMode yes' ubuntu@$ip "sudo apt-get install -y ${packages}"
    abortIfNonZero $? "[${container}] container apt-get install ${packages}"

    if [ -n "${customCommands}" ]; then
        echo "info: Running customCommands command: ${customCommands}"
        ssh -o 'StrictHostKeyChecking no' -o 'BatchMode yes' ubuntu@$ip "${customCommands}"
        abortIfNonZero $? "[${container}] container customCommands command /${customCommands}/"
    fi

    echo "info: stopping ${container} container"
    sudo lxc-stop -k -n $container
    abortIfNonZero $? "[${container}] lxc-stop"
}

function lxcConfigBuildPacks() {
    echo "info: Initializing build-pack: ${buildPack}"
    for buildPack in $(ls -1 ../build-packs); do
        packages="$(cat ../build-packs/$buildPack/container-packages 2>/dev/null)"
        customCommands="$(cat ../build-packs/$buildPack/container-custom-commands 2>/dev/null)"
        lxcConfigBuildPack "${buildPack}" "${packages}" "${customCommands}"
    done
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

source libfns.sh

installGo
installLxc
setupBtrfs
gitLinkage
buildEnv
lxcInitBase
lxcConfigBase
lxcConfigBuildPacks
rsyslogLoggingListeners

exit 0

