function installLxc() {
    echo 'info: You must have a supported version of lxc installed (as of 2013-07-02, `buntu comes with 0.7.x by default, we require is 0.9.0 or greater)'
    echo 'info: Adding LXC PPA'
    sudo add-apt-repository -y ppa:ubuntu-lxc/daily
    rc=$?
    test $rc -ne 0 && echo "error: command 'sudo add-apt-repository -y ppa:ubuntu-lxc/daily' exited with non-zero status: ${rc}" 1>&2 && exit $rc
    sudo apt-get update
    rc=$?
    test $rc -ne 0 && echo "error: command 'sudo add-get update' exited with non-zero status: ${rc}" 1>&2 && exit $rc
    sudo apt-get install -y lxc lxc-templates
    rc=$?
    test $rc -ne 0 && echo "error: command 'apt-get install -y ${required}' exited with non-zero status: ${rc}" 1>&2 && exit $rc

    echo 'info: LXC version should be 0.9.0 or greater:'
    echo "Installed version $(lxc-version) (should be >= 0.9.0)"

    required='btrfs-tools git mercurial bzr build-essential bzip2 daemontools lxc lxc-templates ntp ntpdate'
    echo "info: Installing required build-server packages: ${required}"
    sudo apt-get install -y $required
    rc=$?
    test $rc -ne 0 && echo "error: command 'apt-get install -y ${required}' exited with non-zero status: ${rc}" 1>&2 && exit $rc

    recommended='aptitude htop iotop unzip screen bzip2 bmon'
    echo "info: Installing recommended packages: ${recommended}"
    sudo apt-get install -y $recommended
    rc=$?
    test $rc -ne 0 && echo "error: command 'apt-get install -y ${recommended}' exited with non-zero status: ${rc}" 1>&2 && exit $rc
}

