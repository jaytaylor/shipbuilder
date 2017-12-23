# ShipBuilder

## About

ShipBuilder is a git-based application deployment and serving system (PaaS) written in Go.

Primary components:

* ShipBuilder command-line client
* ShipBuilder server
* Container management (LXC 2.x)
* HTTP load balancer (HAProxy)

## Requirements

The server has been tested and verified compatible with **Ubuntu 16.04**.

Releases may be [downloaded](https://github.com/jaytaylor/shipbuilder/releases), or built on a Ubuntu Linux or macOS machine, provided the following are installed and available in the build environment:

* [golang v1.9+](https://golang.org/dl/)
* git and bzr clients
* [go-bindata](https://github.com/jteeuwen/go-bindata) (`go get -u github.com/jteeuwen/go-bindata/...`)
* fpm (for building debs and RPMs, automatic installation available via `make deps`)
* [daemontools v0.76+](https://github.com/daemontools/daemontools) (for `envdir`)
* Amazon AWS credentials + an s3 bucket

## Build Packs

Any server application can be run on ShipBuilder, but it will need a corresponding build-pack! The current supported build-packs are:

* `python` - Any python 2.x app
* `nodejs` - Node.js apps
* `java8-mvn` - Java 8 + Maven
* `java9-mvn` - Java 9 + Maven
* `scala-sbt` - Scala SBT applications and projects
* `playframework2` - Play-framework 2.1.x

## Server Installation

See [SERVER.md](https://github.com/jaytaylor/shipbuilder/blob/master/SERVER.md)

TODO 2017-10-24: Create additional buildpack provider which uses FS with bindata as a fallthrough, to enable real-time overrides without recompile.

TODO: `lxd init` ??

TODO: 2017-12-03: Fix sb-server /etc/shipbuilder dir permissions to disallow other users from viewing the directory contents (downgrade 3rd party perms).

TODO: 2017-12-04: Fix port allocation bug triggered by blind port incrementing, grep for '// Then attempt start it again.' for relevant section of cmd_deploy.go.

TODO: 2017-12-16: Figure out why shutdown_container.py isn't purging iptables rules.

Why isn't the LB gettng updated hap configs?

Also: Revisit git push weirdness / workaround hacks.

TODO: Test rollbacks.
TODO: Automatically scrub old app images from slaves.

TODO: Additional protection against dyno port conflicts via checking against running containers on the host during launch in container_start.py.

TODO: Disable remaining services in ubuntu container, e.g.:

TODO: Package SB-logger as a standalone program and stop embedding the full sb binary.  Security practice improvement.

/sbin/init
/lib/systemd/systemd-journald
/lib/systemd/systemd-udevd
/lib/systemd/systemd-logind
/usr/bin/dbus-daemon --system --address=systemd: --nofork --nopidfile --systemd-activation
/sbin/dhclient -1 -v -pf /run/dhclient.eth0.pid -lf /var/lib/dhcp/dhclient.eth0.leases -I -df /var/lib/dhcp/dhclient6.eth0.leases eth0
/sbin/agetty --noclear --keep-baud console 115200 38400 9600 linux
[ssh] <defunct>
/bin/sh /usr/lib/apt/apt.systemd.daily install
/bin/sh /usr/lib/apt/apt.systemd.daily lock_is_held install
/usr/bin/python3 /usr/bin/unattended-upgrade


---

Note: it's now recommended to ensure $sbHost is set to a domain name.. example: install/node.sh 2nd ssh cmd.

TOOD: PORT ALLOCATION BUG - could be caused by the tmp.sh during deploy; when there's an error it blindly tries incrementing the port...

2017-12-17: =Idea=
What about tracking user activity logging + queries w/ fields: USERNAME .  Remember how USERNAME is a difficult thing to infer with the present iteration of shipbuilder.  Perhaps make it a pluggable "Addon" or "Module", "Dynamic Plugin Module, etc.  One is the current scheme of not caring about or handling anything.  Maybe it's a plugin which simply enforces that the other spydaddy plugin isn't installed?  Then there is the most granular scenario of the current username where both the username, timestamp, and argv are embedded alongside a system account producing start/stop(/error?) messages. Finally, consider the middleground of not forcing users to get their own accounts, this exists as a clean subset of the more complete solution.

(Also: LDAP integration?)

TODO: 2017-12-18 (Mon) Fix needed for SB client exiting with 0 status code even when connecting to sb-server failed.

TODO: 2017-12-20: SB client can be fixed by adding 'ruok' equivalent in client.go.

TODO: 2017-12-12: Make backup of HAProxy cfg before overwriting, then restore orig config if hap svc restart fails.

## Client

See [CLIENT.md](https://github.com/jaytaylor/shipbuilder/blob/master/CLIENT.md)

TODO 2017-10-15: Migrate client commands to `cli.v2`.

## Creating your first app

All applications need a `Procfile`.  In ShipBuilder, these are 100% compatible with [Heroku's Procfiles (documentation)](https://devcenter.heroku.com/articles/procfile).

See [TUTORIAL.md](https://github.com/jaytaylor/shipbuilder/blob/master/TUTORIAL.md)

## Development

Sample development workflow:

1. Make local edits
2. Run:
```bash
make clean deb \
    && rsync -azve ssh dist/*.deb dev-host.lan:/tmp/ \
    && ssh dev-host.lan /bin/sh -c \
        'set -e && cd /tmp/ ' \
        '&& sudo --non-interactive dpkg -i *.deb && rm *.deb ' \
        '&& sudo --non-interactive systemctl daemon-reload ' \
        '&& sudo --non-interactive systemctl restart shipbuilder'
```

## Thanks

Thank you to [SendHub](https://www.sendhub.com) for supporting the initial development of this project.

