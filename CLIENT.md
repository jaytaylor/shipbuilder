## Client Commands

Build the client by running `make`, and run it with `shipbuilder client [command]`.

Note:

*   Any command that takes an [application-name] either gets the application name from the current directory or it must be specified with `-a<application name>`.

## System-wide commands

**apps:list**

    apps[:list?]

Lists all applications.

**lb:add**

    lb:add [address]..

Add one or more new load balancers to the system. Updates the load balancer config.

**lb:list**

    lb[:list?]

List all the load balancers.

**lb:remove**

    lb:remove [address]..

Remove one or more load balancers from the system. Updates the load balancer config.

**apps:health**

    [apps:?]health

Display detailed output on the health of each process-type for each app.

IMPORTANT: If this command is run while a deployment is in progress, then it will hang until after the deployment is finished.

**nodes:add**

    nodes:add [address]..

Add one or more nodes to the system (a node hosts the containers running the actual apps).

**nodes:add**

    nodes:add [address]..

Add one or more nodes to the system (a node hosts the containers running the actual apps).

**nodes:list**

    nodes[:list?]

Display listing of all nodes and processes runnin on each of them.

**nodes:remove**

    nodes:remove [address]..

Remove one or more nodes from the system.

## Application-specific commands

**apps:create**

    [apps:]create [application-name] [buildpack]

Alternative flag combinations:
[apps:]create -a[application-name][buildpack]

Create an appication named `name` with the build pack `buildpack`. Available buildpacks are:

*   python
*   nodejs
*   scala-sbt
*   playframework2

**apps:clone**

    [apps:]clone [old-application-name] [new-application-name]

Alternative flag combinations:
[apps:]clone -o[old-application-name] -n[new-application-name][apps:]clone --oldName=[old-application-name] --newName=[new-application-name]

Clone (copy) an application with it's config and processes settings into a new app.

**apps:destroy**

    [apps:]destroy -a[application-name]

Destroy the app with the name `name`. This permanently and irreversibly deletes the application configuration, the base container image, and all prior releases archived on S3.

**config:list**

    config[:list] -a[application-name]

Show all the configuration entries for an application.

**config:get**

    config:get -a[application-name] variable-name

Return the configuration entry for an application and variable name.

**config:set**

    config:set -a[application-name] [variable-name]=[variable-value]..

Set one or more configuration environment variables for the named application. Redeploys the app.

There is also a `--deferred=1`/`-d1` flag which can be passed to cause the config change to take effect the next time the app is deployed (avoids the default immediate redeploy).

**config:remove**

    config:remove -a[application-name] [variable-name]..

Delete one or more configuration environment variables for the named application. Redeploys the app.

There is also a `--deferred=1`/`-d1` flag which can be passed to cause the config change to take effect the next time the app is deployed (avoids the default immediate redeploy).

**deploy**

    deploy -a[application-name] revision

Deploy an application at the given revision (the revision must be available in the local git repository).

**domains:add**

    domains:add -a[application-name] [domain-name]..

Add one or more domains to an application. Updates and reloads the load-balancer immediately; Does NOT redeploy the app.

**domains:list**

    domains:list -a[application-name]

List the domains for an application.

**domains:remove**

    domains:remove -a[application-name] [domain-name]..

Remove one or more domains from an application. Does NOT redeploy the app.

**logs**

    logs -a[application-name]

Display the logs for an application. _Not Implemented_

**maintenance:off**

    maintenance:off -a[application-name]

Turns off maintenance mode for an application.

**maintenance:on**

    maintenance:on -a[application-name]

Turns on maintenance mode for an application.

**maintenance:status**

    maintenance[:status?] -a[application-name]

Gets the current maintenance status for an application. Status values are "on" or "off".

**maintenance:url**

    maintenance:url -a[application-name] [url?]

If `url` is empty, the current maintenance page URL is shown.
If `url` is not empty, will sets the environment variable `MAINTENANCE_PAGE_URL`, which will be used when maintenance-mode is "on". No redeploy required.
Alternatively, you can also use config:set to a similar effect, with the addition of a full redeploy, e.g.:

    sb config:set -aMyApp MAINTENANCE_PAGE_URL='http://example.com/foo/bar.html'

**pre-receive**

    pre-receive directory old-revision new-revision reference

Internal command automatically invoked by the git repo on pre-receive.

**post-receive**

    post-receive directory old-revision new-revision reference

Internal command automatically invoked by the git repo on post-receive.

**privatekey:get**

    privatekey[:get?] -a[application-name]

Get the private SSH key for an app.

**privatekey:set**

    privatekey:set -a[application-name] [REALLY-LONG-PRIVATE-KEY-STRING-ALL-ON-ONE-LINE-WITH-NO-DASHES]

Set the private SSH key for an app (so dependencies and submodules from private repositories can be retrieved).

**privatekey:remove**

    privatekey:remove -a[application-name]

Remove the currently set priate SSH key for an app.

**ps:list**

    ps[:list?] -a[application-name]

List the goal and actual running instances of an application.

**ps:restart**

    ps:restart -a[application-name] [process-type-x]..

Restart one or more process types for the app. Does NOT trigger a redeploy.

**ps:start**

    ps:start -a[application-name] [process-type-x]..

Launch the service for one or more process types of the app. Does NOT trigger a redeploy.

**ps:status**

    ps:status -a[application-name] [process-type-x?]..

Launch the service for one or more process types of the app. Does NOT trigger a redeploy.

**ps:stop**

    ps:stop -a[application-name] [process-type-x]..

Stop the service for one or more process types of the app. Does NOT trigger a redeploy.

**ps:scale**

    ps:scale -a[application-name] [process-type]=#num#..

Update the number of dyno instances for one or more process types. Redeploys the app.

**redeploy**

    redeploy -a[application-name]

Trigger a full redeploy for the app.

**releases:info**

    releases:info -a[application-name] [version]

Get the release information for an application at the given version. _Not yet implemented_

**releases:list**

    releases[:list?] -a[application-name]

List the most recent 15 releases for an application.

**reset**

    reset -a[application-name]

Reset an the base container for an applications. This will force all dependencies to be freshly downloaded and built during the next deploy.

**rollback**

    rollback -a[application-name] [version]

Rollback an application to a specific version. Note: Version is not optional.

**run**

    run -a[application-name] [shell-command?]

Starts up a temporary container and hooks the current connection to a shell. If `shell-command` is omitted, by default a bash shell will launched.

**runtime:tests**

    runtime[:]tests

Runs and reports the status of ShipBuilder server system and environment checks and tests. Including: - S3 read/write capability to the configured bucket.

**sys:zfscleanup**

    sys:zfs[cleanup?]

System command: Manually run the ZFS maintenance cleanup task (NB: this runs automatically via a ShipBuilder cron @ 7:30PM UTC).

**sys:snapshotscleanup**

    sys:snapshots[cleanup?]

System command: Manually run the snapshot cleanup task (NB: this runs automatically via a ShipBuilder cron every 2 hours).

**sys:ntpsync**

    sys:ntp[sync?]

System command: Manually run NTP update/sync across all members of the ShipBuilder cluster (i.e. SB server, load-balancers, and nodes) (NB: this runs automatically via a ShipBuilder cron at the top of every hour).

## Project Compilation

Requirements:

*   go-lang v1.2 or v1.1
*   daemontools (the package which contains `envdir`)
*   ssh
*   git
*   bzr

First set up your env:

```bash
echo 'sb.sendhub.com' > env/SB_SSH_HOST
echo 'admin:password' > env/SB_HAPROXY_CREDENTIALS
echo 'true' > env/SB_HAPROXY_STATS
echo "$HOME/.ssh/id_rsa" > env/SB_SSH_KEY
```

Build the client:

```bash
make clean build
```

The resulting binary will be created under ./shipbuilder, e.g.:

```bash
./shipbuilder/shipbuilder-darwin
./shipbuilder/shipbuilder-linux
```

Deploy to SB_SSH_HOST:

```bash
rsync -azve ssh ~/go/src ${SB_SSH_HOST}:~/go/
ssh ${SB_SSH_HOST} bash -c 'make clean deb && sudo dpkg -i dist/shipbuilder_*.deb && sudo systemctl restart shipbuilder'
```

## Setting a maintenance page URL

Set your own custom maintenance page URL to be displayed while the app is in maintenance mode.

    sb config:set -aMyApp MAINTENANCE_PAGE_URL='http://example.com/foo/bar.html'

## Setting deploy-hooks URLs

Set a deploy-hook URL to enable things like HipChat room notifications.

    sb config:set -aMyApp SB_DEPLOYHOOKS_HTTP_URL='https://api.hipchat.com/v1/rooms/message?auth_token=<THE_TOKEN>&room_id=<THE_ROOM>'

Multiple deploy-hook URLs can be set by adding _"\_0"_, _"\_1"_, .., _"\_N"_ to the `SB_DEPLOYHOOKS_HTTP_URL` environment variable.

Note: Gaps are permitted in the trailing number _N_. Whenever _N_ is incremented 10 times without finding any values, the env var search halts.

    sb config:set -aMyApp \
        SB_DEPLOYHOOKS_HTTP_URL_0='https://api.hipchat.com/v1/...' \
        SB_DEPLOYHOOKS_HTTP_URL_1='https://hooks.slack.com/services/...'

### Supported deploy-hook integrations

#### HipChat

Only needs the URL with bundled API token.

#### Slack

Only needs the URL with bundled API token.

#### New Relic

The New Relic integration uses an HTTP POST call with the NR API key sent as a special header.

For it to work, the `SB_NEWRELIC_API_KEY` app environment variable must be set.

#### Datadog

The datadog integration requires that 2 app environment variables be set:

*   `SB_DATADOG_API_KEY`
*   `SB_DATADOG_APP_KEY`

These can be found and managed from the [Datadog console](https://app.datadoghq.com/account/settings#api).

## ShipBuilder Client Configuration Overrides

temporary `env` config overrides are possible, just prefix the variable=value before invoking the client:

    $ SB_SSH_HOST=sb-staging.sendhub.com ./shipbuilder config -aMyApp
    info: environmental override detected for SB_SSH_HOST: sb-staging.sendhub.com
    ..
