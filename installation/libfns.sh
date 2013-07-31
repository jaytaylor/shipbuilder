function abortIfNonZero() {
    rc=$1
    details=$2
    test $rc -ne 0 && echo "error: ${details} exited with non-zero status ${rc}" 1>&2 && exit $rc
}

function installLxc() {
    echo 'info: You must have a supported version of lxc installed (as of 2013-07-02, `buntu comes with 0.7.x by default, we require is 0.9.0 or greater)'
    echo 'info: Adding LXC PPA'
    sudo add-apt-repository -y ppa:ubuntu-lxc/daily
    abortIfNonZero $? "command 'sudo add-apt-repository -y ppa:ubuntu-lxc/daily'"
    sudo apt-get update
    abortIfNonZero $? "command 'sudo add-get update'"
    sudo apt-get install -y lxc lxc-templates
    abortIfNonZero $? "command 'apt-get install -y ${required}'"

    echo 'info: LXC version should be 0.9.0 or greater:'
    echo "Installed version $(lxc-version) (should be >= 0.9.0)"

    required='btrfs-tools git mercurial bzr build-essential bzip2 daemontools lxc lxc-templates ntp ntpdate'
    echo "info: Installing required build-server packages: ${required}"
    sudo apt-get install -y $required
    abortIfNonZero $? "command 'apt-get install -y ${required}'"

    recommended='aptitude htop iotop unzip screen bzip2 bmon'
    echo "info: Installing recommended packages: ${recommended}"
    sudo apt-get install -y $recommended
    abortIfNonZero $? "command 'apt-get install -y ${recommended}'"
}

function verifySshAndSudoForHosts() {
    # @param $1 string. List of space-delimited SSH connection strings.
    local sshHosts="$1"
    echo "info: Verifying ssh and sudo access for $(echo "${sshHosts}" | tr ' ' '\n' | grep -v '^ *$' | wc -l | sed 's/^[ \t]*//g') hosts"
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

