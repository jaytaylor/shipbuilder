ShipBuilder
===========

Additional information is available at [https://shipbuilder.gigawatt.io](shipbuilder.gigawatt.io)

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

* Ubuntu 13.10, 13.04, or 12.04 (tested and verified compatible)
* go-lang v1.2 or v1.1
* envdir (linux: `apt-get install daemontools`, os-x: `brew install daemontools`)
* git and bzr clients
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

Getting Help
------------
Have a question? Want some help? You can reach shipbuilder experts any of the following ways:

Discussion List: [ShipBuilder Google Group](https://groups.google.com/forum/#!forum/shipbuilder)
IRC: [#shipbuilder on FreeNode](irc://chat.freenode.node/shipbuilder)
Twitter: [ShipBuilderIO](https://twitter.com/ShipBuilderIO)

Or open a GitHub issue.

Contributing
------------
1. "Fork"
2. Make a feature branch.
3. Do your commits
4. Send "pull request". This can be
	1. A github pull request
	2. A issue with a pointer to your publicly readable git repo
	3. An email to me with a pointer to your publicly readable git repo

Thanks
------
Thank you to [SendHub](https://www.sendhub.com) for supporting the initial development of this project.

