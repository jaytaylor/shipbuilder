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

Note: it's now recommended to ensure $sbHost is set to a domain name.. example: install/node.sh 2nd ssh cmd.

TOOD: PORT ALLOCATION BUG - could be caused by the tmp.sh during deploy; when there's an error it blindly tries incrementing the port...

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

