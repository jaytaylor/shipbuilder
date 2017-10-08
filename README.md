ShipBuilder
===========

Additional information is available at [https://shipbuilder.io](http://shipbuilder.io)

About
-----
ShipBuilder is a git-based application deployment and serving system written in Go.

Primary components:

* ShipBuilder command-line client
* ShipBuilder server
* Container management (LXC)
* HTTP load balancer (HAProxy)

Build Packs
-----------
Any app server can run on ShipBuilder, but it will need a build-pack! The current build-packs are:
* `python` - Any python app
* `nodejs` - Node.js apps
* `scala-sbt` - Scala SBT applications and projects
* `playframework2` - Play-framework 2.1.x

Requirements:

* Ubuntu 16.04 (tested and verified compatible)
* golang v1.9+
* git and bzr clients
* fpm (for building debs and RPMs, automatic installation available via `make deps`)
* Amazon AWS credentials + an s3 bucket

Server Installation
-------------------

See [SERVER.md](https://github.com/jaytaylor/shipbuilder/blob/master/SERVER.md)

Client
------

See [CLIENT.md](https://github.com/jaytaylor/shipbuilder/blob/master/CLIENT.md)

Creating your first app
-----------------------

All applications need a `Procfile`.  In ShipBuilder, these are 100% compatible with [Heroku's Procfiles (documentation)](https://devcenter.heroku.com/articles/procfile).

See [TUTORIAL.md](https://github.com/jaytaylor/shipbuilder/blob/master/TUTORIAL.md)

Development
-----------

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

Thanks
------
Thank you to [SendHub](https://www.sendhub.com) for supporting the initial development of this project.

