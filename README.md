ShipBuilder
===========

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
Any app server can be run on ShipBuilder, but if will need a build-pack! Current build-packs are:
* `python` - Any python app
* `playframework2` - Play-framework 2.1.x

Requirements:

* Ubuntu 12.04 or 13.04 (tested and verified compatible)
* go-lang v1.1
* envdir (linux: `apt-get install daemontools`, os-x: `brew install daemontools`)
* Amazon AWS credentials + an s3 bucket

Server Installation
----------------------------

See [SERVER.md](https://github.com/sendhub/shipbuilder/blob/master/SERVER.md)

Client
------

See [CLIENT.md](https://github.com/sendhub/shipbuilder/blob/master/CLIENT.md)

Creating your first app
-----------------------

See [TUTORIAL.md](https://github.com/sendhub/shipbuilder/blob/master/TUTORIAL.md)
