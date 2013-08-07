function abortIfNonZero() {
    # @param $1 return code/exit status (e.g. $?)
    # @param $2 error message if exit status was non-zero.
    local rc=$1
    local what=$2
    test $rc -ne 0 && echo "error: ${what} exited with non-zero status ${rc}" 1>&2 #&& exit $rc
}

function autoDetectServer() {
    # Attempts to auto-detect the server host by reading the contents of ../env/SB_SSH_HOST.
    if [ -r "../env/SB_SSH_HOST" ]; then
        sbHost=$(head -n1 ../env/SB_SSH_HOST)
        if [ -n "${sbHost}" ]; then
            echo "info: auto-detected shipbuilder host: ${sbHost}"
        fi
    else
        echo 'warn: server auto-detection failed: no such file: ../env/SB_SSH_HOST' 1>&2
    fi
}

function verifySshAndSudoForHosts() {
    # @param $1 string. List of space-delimited SSH connection strings.
    local sshHosts="$1"
    echo "info: verifying ssh and sudo access for $(echo "${sshHosts}" | tr ' ' '\n' | grep -v '^ *$' | wc -l | sed 's/^[ \t]*//g') hosts"
    for sshHost in $(echo "${sshHosts}"); do
        echo -n "info:     testing host ${sshHost} .. "
        result=$(ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' -o 'ConnectTimeout 15' -q $sshHost 'sudo -n echo "succeeded" 2>/dev/null')
        rc=$?
        if [ $rc -ne 0 ]; then
            echo 'failed'
            echo "error: ssh connection test failed for host: ${sshHost}" 1>&2
            exit 1
        fi
        if [ -z "${result}" ]; then
            echo 'failed'
            echo "error: sudo access test failed for host: ${sshHost}" 1>&2
            exit 1
        fi
        echo 'succeeded'
    done
}

function initSbServerKeys() {
    # @precondition $sbHost must not be empty.
    test -z "${sbHost}" && echo 'error: initSbServerKeys(): required parameter $sbHost cannot be empty' 1>&2 && exit 1
    echo "info: checking SB server \"${sbHost}\" SSH keys, will generate if missing"

    ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $sbHost '/bin/bash -c '"'"'
    echo "remote: info: setting up pub/private SSH keys so that root and main users can SSH in to either account"
    function abortIfNonZero() {
        local rc=$1
        local what=$2
        test $rc -ne 0 && echo "remote: error: ${what} exited with non-zero status ${rc}" && exit $rc
    }
    if ! test -e ~/.ssh/id_rsa.pub; then
        echo "remote: info: generating a new private/public key-pair for main user"
        rm -f ~/.ssh/id_*
        abortIfNonZero $? "removing old keys failed"
        ssh-keygen -f ~/.ssh/id_rsa -t rsa -N ""
        abortIfNonZero $? "ssh-keygen command failed"
    fi
    if test -z "$(grep "$(cat ~/.ssh/id_rsa.pub)" ~/.ssh/authorized_keys)"; then
        echo "remote: info: adding main user to main user authorized_keys"
        cat ~/.ssh/id_rsa.pub >> ~/.ssh/authorized_keys
        abortIfNonZero $? "appending public-key to authorized_keys command"
        chmod 600 ~/.ssh/authorized_keys
        abortIfNonZero $? "chmod 600 ~/.ssh/authorized_keys command"
    fi
    if sudo test -z "$(sudo grep "$(cat ~/.ssh/id_rsa.pub)" /root/.ssh/authorized_keys)"; then
        echo "remote: info: adding main user to root user authorized_keys"
        cat ~/.ssh/id_rsa.pub | sudo tee -a /root/.ssh/authorized_keys >/dev/null
        abortIfNonZero $? "appending public-key to authorized_keys command"
        sudo chmod 600 /root/.ssh/authorized_keys
        abortIfNonZero $? "chmod 600 /root/.ssh/authorized_keys command"
    fi

    if ! sudo test -e /root/.ssh/id_rsa.pub; then
        echo "remote: info: generating a new private/public key-pair for root user"
        sudo rm -f /root/.ssh/id_*
        abortIfNonZero $? "removing old keys failed"
        sudo ssh-keygen -f /root/.ssh/id_rsa -t rsa -N ""
        abortIfNonZero $? "ssh-keygen command failed"
    fi
    if sudo test -z "$(sudo grep "$(sudo cat /root/.ssh/id_rsa.pub)" /root/.ssh/authorized_keys)"; then
        echo "remote: info: adding root to root user authorized_keys"
        sudo cat /root/.ssh/id_rsa.pub | sudo tee -a /root/.ssh/authorized_keys >/dev/null
        abortIfNonZero $? "appending public-key to authorized_keys command"
        sudo chmod 600 /root/.ssh/authorized_keys
        abortIfNonZero $? "chmod 600 /root/.ssh/authorized_keys command"
    fi
    if test -z "$(grep "$(sudo cat /root/.ssh/id_rsa.pub)" ~/.ssh/authorized_keys)"; then
        echo "remote: info: adding root to main user authorized_keys"
        sudo cat /root/.ssh/id_rsa.pub >> ~/.ssh/authorized_keys
        abortIfNonZero $? "appending public-key to authorized_keys command"
        sudo chmod 600 ~/.ssh/authorized_keys
        abortIfNonZero $? "chmod 600 ~/.ssh/authorized_keys command"
    fi'"'"
    abortIfNonZero $? "ssh key initialization"
}

function getSbServerPublicKeys() {
    test -z "${sbHost}" && echo 'error: getSbServerPublicKeys(): missing required parameter: SSH hostname' 1>&2 && exit 1

    initSbServerKeys

    echo "info: retrieving public-keys from shipbuilder server: ${sbHost}"

    local pubKeys=$(ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $sbHost 'cat ~/.ssh/id_rsa.pub && echo "." && sudo cat /root/.ssh/id_rsa.pub')
    abortIfNonZero $? 'SSH public-key retrieval failed'

    unprivilegedPubKey=$(echo "${pubKeys}" | grep --before 100 '^\.$' | grep -v '^\.$')
    rootPubKey=$(echo "${pubKeys}" | grep --after 100 '^\.$' | grep -v '^\.$')

    if [ -z "${unprivilegedPubKey}" ]; then
        echo 'error: failed to obtain build-server public-key for unprivileged user' 1>&2
        exit 1
    fi
    echo "info: obtained unprivileged public-key: ${unprivilegedPubKey}"
    if [ -z "${rootPubKey}" ]; then
        echo 'error: failed to obtain build-server public-key for root user' 1>&2
        exit 1
    fi
    echo "info: obtained root public-key: ${rootPubKey}"
}

function installAccessForSshHost() {
    # @precondition Variable $sshKeysCommand must be initialized and not empty.
    # @param $1 SSH connection string (e.g. user@host)
    local sshHost=$1

    if [ -z "${unprivilegedPubKey}" ] || [ -z "${rootPubKey}" ]; then
        getSbServerPublicKeys
    fi

    test -z "${sshHost}" && echo 'error: installAccessForSshHost(): missing required parameter: SSH hostname' 1>&2 && exit 1

    echo "info: setting up remote access from build-server to host: ${sshHost}"
    ssh -o 'BatchMode yes' -o 'StrictHostKeyChecking no' $sshHost '/bin/bash -c '"'"'
    function abortIfNonZero() {
        local rc=$1
        local what=$2
        test $rc -ne 0 && echo "remote: error: ${what} exited with non-zero status ${rc}" && exit $rc
    }
    if test -z "$(sudo grep "'"${unprivilegedPubKey}"'" ~/.ssh/authorized_keys)" || test -z "$(sudo grep "'"${unprivilegedPubKey}"'" ~/.ssh/authorized_keys)"; then
        echo -e "'"${unprivilegedPubKey}\n${rootPubKey}"'" >> ~/.ssh/authorized_keys
        abortIfNonZero $? "appending public-keys to authorized_keys command"
        chmod 600 ~/.ssh/authorized_keys
        abortIfNonZero $? "chmod 600 ~/.ssh/authorized_keys command"
    fi
    if sudo test -z "$(sudo grep "'"${unprivilegedPubKey}"'" /root/.ssh/authorized_keys)" || sudo test -z "$(sudo grep "'"${unprivilegedPubKey}"'" /root/.ssh/authorized_keys)"; then
        echo -e "'"${unprivilegedPubKey}\n${rootPubKey}"'" | sudo tee -a /root/.ssh/authorized_keys >/dev/null
        abortIfNonZero $? "appending public-keys to authorized_keys command"
        sudo chmod 600 /root/.ssh/authorized_keys
        abortIfNonZero $? "chmod 600 /root/.ssh/authorized_keys command"
    fi
    '"'"
    abortIfNonZero $? "ssh access installation failed for host ${sshHost}"
}

function installLxc() {
    echo 'info: a supported version of lxc must be installed (as of 2013-07-02, `buntu comes with 0.7.x by default, we require is 0.9.0 or greater)'
    echo 'info: adding lxc daily ppa'
    sudo add-apt-repository -y ppa:ubuntu-lxc/daily
    abortIfNonZero $? "command 'sudo add-apt-repository -y ppa:ubuntu-lxc/daily'"
    sudo apt-get update
    abortIfNonZero $? "command 'sudo add-get update'"
    sudo apt-get install -y lxc lxc-templates
    abortIfNonZero $? "command 'apt-get install -y ${required}'"

    echo "info: installed version $(lxc-version) (should be >= 0.9.0)"

    local required='btrfs-tools git mercurial bzr build-essential bzip2 daemontools lxc lxc-templates ntp ntpdate'
    echo "info: installing required build-server packages: ${required}"
    sudo apt-get install -y $required
    abortIfNonZero $? "command 'apt-get install -y ${required}'"

    local recommended='aptitude htop iotop unzip screen bzip2 bmon'
    echo "info: installing recommended packages: ${recommended}"
    sudo apt-get install -y $recommended
    abortIfNonZero $? "command 'apt-get install -y ${recommended}'"
}

function prepareNode() {
    # @param $1 device to format and use for new mount.
    device=$1
    test -z "${device}" && echo 'error: missing required parameter: -d [device]' 1>&2 && exit 1
    test ! -e "${device}" && echo "error: unrecognized device '${device}'" 1>&2 && exit 1

    installLxc

    echo "info: attempting to unmount /mnt and ${device} to be safe"
    sudo umount /mnt 1>&2 2>/dev/null
    sudo umount $device 1>&2 2>/dev/null

    # Try to temporarily mount the device to get an accurate FS-type reading.
    sudo mount $device /mnt 1>&2 2>/dev/null
    fs=$(sudo df -T $device | tail -n1 | sed 's/[ \t]\+/ /g' | cut -d' ' -f2)
    test -z "${fs}" && echo "error: failed to determine FS type for ${device}" 1>&2 && exit 1
    sudo umount $device 1>&2 2>/dev/null

    echo "info: existing fs type on ${device} is ${fs}"
    if [ "${fs}" = "btrfs" ]; then
        echo "info: ${device} is already formatted with btrfs"
    else
        echo "info: formatting ${device} with btrfs"
        sudo mkfs.btrfs $device
        abortIfNonZero $? "mkfs.btrfs ${device}"
    fi

    if ! [ -d /mnt/build ]; then
        echo 'info: creating /mnt/build mount point'
        sudo mkdir -p /mnt/build
        abortIfNonZero $? "creating /mnt/build"
    fi

    echo 'info: updating /etc/fstab to map /mnt/build to the btrfs device'
    if [ -z "$(grep "$(echo $device | sed 's:/:\\/:g')" /etc/fstab)" ]; then
        echo 'info: adding new fstab entry'
        echo "${device} /mnt/build auto defaults 0 0" | sudo tee -a /etc/fstab >/dev/null
        abortIfNonZero $? "fstab add"
    else
        echo 'info: editing existing fstab entry'
        sudo sed -i "s-^\s*${device}\s\+.*\$-${device} /mnt/build auto defaults 0 0-" /etc/fstab
        abortIfNonZero $? "fstab edit"
    fi

    echo "info: mounting device ${device}"
    sudo mount $device
    abortIfNonZero $? "mounting ${device}"

    if [ -d /var/lib/lxc ] && ! [ -e /mnt/build/lxc ]; then
        echo 'info: creating and linking /mnt/build/lxc folder'
        sudo mv /{var/lib,mnt/build}/lxc
        abortIfNonZero $? "lxc directory migration"
        sudo ln -s /mnt/build/lxc /var/lib/lxc
        abortIfNonZero $? "lxc directory symlink"
    fi

    if ! [ -d /mnt/build/lxc ]; then
        echo 'info: attempting to create missing /mnt/build/lxc'
        sudo mkdir /mnt/build/lxc
        abortIfNonZero $? "lxc directory creation"
    fi

    if ! [ -e /var/lib/lxc ]; then
        echo 'info: attemtping to symlink missing /var/lib/lxc to /mnt/build/lxc'
        sudo ln -s /mnt/build/lxc /var/lib/lxc
        abortIfNonZero $? "lxc directory symlink 2nd attempt"
    fi
}

function prepareLoadBalancer() {
    # @param $1 ssl certificate base filename (without path).
    certFile=/tmp/$1

    version=$(lsb_release -a 2>/dev/null | grep "Release" | grep -o "[0-9\.]\+$")

    if [ "${version}" = "12.04" ]; then
        ppa=ppa:vbernat/haproxy-1.5
    elif [ "${version}" = "13.04" ]; then
        ppa=ppa:nilya/haproxy-1.5
    else
        echo "error: unrecognized version of ubuntu: ${version}" 1>&2 && exit 1
    fi
    echo "info: adding ppa repository for ${version}: ${ppa}"
    sudo apt-add-repository -y ${ppa}
    abortIfNonZero $? "adding apt repository ppa ${ppa}"

    required="haproxy"
    echo "info: installing required packages: ${required}"
    sudo apt-get update
    abortIfNonZero $? "updating apt"
    sudo apt-get install -y $required
    abortIfNonZero $? "apt-get install ${required}"

    optional="vim-haproxy"
    echo "info: installing optional packages: ${optional}"
    sudo apt-get install -y $optional
    abortIfNonZero $? "apt-get install ${optional}"

    if [ -r "${certFile}" ]; then
        if ! [ -d "/etc/haproxy/certs.d" ]; then
            sudo mkdir /etc/haproxy/certs.d 2>/dev/null
            abortIfNonZero $? "creating /etc/haproxy/certs.d directory"
        fi

        echo "info: installing ssl certificate to /etc/haproxy/certs.d"
        sudo mv $certFile /etc/haproxy/certs.d/
        abortIfNonZero $? "moving certificate to /etc/haproxy/certs.d"

        sudo chmod 400 /etc/haproxy/certs.d/$(echo $certFile | sed "s/^.*\/\(.*\)$/\1/")
        abortIfNonZero $? "chmod 400 /etc/haproxy/certs.d/<cert-file>"

        sudo chown -R haproxy:haproxy /etc/haproxy/certs.d
        abortIfNonZero $? "chown haproxy:haproxy /etc/haproxy/certs.d"

    else
        echo "warn: no certificate file was provided, ssl support will not be available" 1>&2
    fi

    echo "info: enabling the HAProxy system service in /etc/default/haproxy"
    sudo sed -i "s/ENABLED=0/ENABLED=1/" /etc/default/haproxy
    abortIfNonZero $? "enabling haproxy service in /dev/default/haproxy"
}

function installGo() {
    if [ -z "$(which go)" ]; then
        echo 'info: installing go 1.1 on the build-server'
        curl --location --silent https://launchpad.net/ubuntu/+archive/primary/+files/golang-src_1.1-1_amd64.deb > /tmp/golang-src_1.1-1_amd64.deb
        curl --location --silent https://launchpad.net/ubuntu/+archive/primary/+files/golang-go_1.1-1_amd64.deb > /tmp/golang-go_1.1-1_amd64.deb
        sudo dpkg -i /tmp/golang-src_1.1-1_amd64.deb /tmp/golang-go_1.1-1_amd64.deb
        mkdir ~/go 2>/dev/null
        echo 'info: adding $GOPATH to ~/.bashrc'
        echo 'export GOPATH=$HOME/go' >> ~/.bashrc
    else
        echo 'info: go already appears to be installed, not going to force it'
    fi
}

function gitLinkage() {
    echo 'info: creating and linking /mnt/build/git -> /git'
    sudo mkdir -p /mnt/build/git 2>/dev/null
    abortIfNonZero $? "creating directory /mnt/build/git"
    sudo chown 777 /mnt/build/git
    abortIfNonZero $? "chown 777 /mnt/build/git"
    sudo ln -s /mnt/build/git /git
    abortIfNonZero $? "symlink /mnt/build/git to /git"
}

function buildEnv() {
    echo 'info: add build servers hostname configuration'
    sudo mkdir /mnt/build/env 2>/dev/null
    abortIfNonZero $? "creating directory /mnt/build/env"
    echo "${sbHost}" | sudo tee /mnt/build/env/SB_SSH_HOST
    abortIfNonZero $? "appending shipbuilder host to /mnt/build/env/SB_SSH_HOST"
}

function rsyslogLoggingListeners() {
    echo 'info: enabling rsyslog listeners'
    echo '# UDP syslog reception.
    $ModLoad imudp
    $UDPServerAddress 0.0.0.0
    $UDPServerRun 514

    # TCP syslog reception.
    $ModLoad imtcp
    $InputTCPServerRun 10514' | sudo tee /etc/rsyslog.d/49-haproxy.conf
    echo 'info: restarting rsyslog'
    sudo service rsyslog restart
    if [ -e /etc/rsyslog.d/haproxy.conf ]; then
        echo 'info: detected existing rsyslog haproxy configuration, will disable it'
        sudo mv /etc/rsyslog.d/haproxy.conf /etc/rsyslog.d-haproxy.conf.disabled
    fi
}

function getContainerIp() {
    local container="$1"
    local allowedAttempts=60
    local i=0
    echo "info: getting container ip-address for name '${container}'"
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
    echo 'info: clear any pre-existing "base" container'
    sudo lxc-stop -k -n base 2>/dev/null
    sudo lxc-destroy -n base 2>/dev/null

    echo 'info: creating base lxc container'
    sudo lxc-create -n base -B btrfs -t ubuntu
    abortIfNonZero $? "lxc-create base"

    echo 'info: configuring base lxc container..'
    sudo lxc-start --daemon -n base
    abortIfNonZero $? "lxc-start base"

    getContainerIp base
}

function lxcConfigBase() {
    echo "info: adding shipbuilder server's public-key to authorized_keys file in base container"
    sudo mkdir /mnt/build/lxc/base/rootfs/home/ubuntu/.ssh
    abortIfNonZero $? "base container .ssh directory"

    sudo cp ~/.ssh/id_rsa.pub /mnt/build/lxc/base/rootfs/home/ubuntu/.ssh/authorized_keys
    abortIfNonZero $? "base container ssh authorized_keys"

    sudo chown -R ubuntu:ubuntu /mnt/build/lxc/base/rootfs/home/ubuntu/.ssh
    abortIfNonZero $? "chown -R ubuntu:ubuntu base container .ssh"

    sudo chmod 700 ubuntu:ubuntu /mnt/build/lxc/base/rootfs/home/ubuntu/.ssh
    abortIfNonZero $? "chmod 700 base container .ssh"

    sudo chmod 600 ubuntu:ubuntu /mnt/build/lxc/base/rootfs/home/ubuntu/.ssh/authorized_keys
    abortIfNonZero $? "chmod 600 base container .ssh/authorized_keys"

    echo 'info: adding the container "ubuntu" user to the sudoers list'
    echo 'ubuntu ALL=(ALL) NOPASSWD: ALL' | sudo tee -a /mnt/build/lxc/base/rootfs/etc/sudoers >/dev/null
    abortIfNonZero $? "adding 'ubuntu' to container sudoers"

    echo 'info: updating apt repositories in container'
    ssh -o 'StrictHostKeyChecking no' -o 'BatchMode yes' ubuntu@$ip "sudo apt-get update"
    abortIfNonZero $? "container apt-get update"

    packages='daemontools git-core curl unzip'
    echo "info: installing packages to base container: ${packages}"
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
    echo "info: creating build-pack ${container} container"
    sudo lxc-clone -s -B btrfs -o base -n $container
    sudo lxc-start -d -n $container
    getContainerIp $container

    echo "info: installing packages to ${container} container: ${packages}"
    ssh -o 'StrictHostKeyChecking no' -o 'BatchMode yes' ubuntu@$ip "sudo apt-get install -y ${packages}"
    abortIfNonZero $? "[${container}] container apt-get install ${packages}"

    if [ -n "${customCommands}" ]; then
        echo "info: running customCommands: ${customCommands}"
        ssh -o 'StrictHostKeyChecking no' -o 'BatchMode yes' ubuntu@$ip "${customCommands}"
        abortIfNonZero $? "[${container}] container customCommands command /${customCommands}/"
    fi

    echo "info: stopping ${container} container"
    sudo lxc-stop -k -n $container
    abortIfNonZero $? "[${container}] lxc-stop"
}

function lxcConfigBuildPacks() {
    for buildPack in $(ls -1 /mnt/build/build-packs); do
        echo "info: initializing build-pack: ${buildPack}"
        packages="$(cat /mnt/build/build-packs/$buildPack/container-packages 2>/dev/null)"
        customCommands="$(cat /mnt/build/build-packs/$buildPack/container-custom-commands 2>/dev/null)"
        lxcConfigBuildPack "${buildPack}" "${packages}" "${customCommands}"
    done
}

function prepareServerPart1() {
    # @param $1 ShipBuilder server hostname or ip-address.
    # @param $2 device to format and use for new mount.
    sbHost=$1
    device=$2
    test -z "${sbHost}" && echo 'error: prepareServer(): missing required parameter: shipbuilder host' 1>&2 && exit 1
    test -z "${device}" && echo 'error: prepareServer(): missing required parameter: device' 1>&2 && exit 1
    prepareNode $device
    installGo
    gitLinkage
    buildEnv
    rsyslogLoggingListeners
}

function prepareServerPart2() {
    lxcInitBase
    lxcConfigBase
    lxcConfigBuildPacks
}
