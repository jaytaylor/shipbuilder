package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

func (this *Server) numDynosAtVersion(applicationName, version string, hostStatusMap *map[string]NodeStatus) (int, error) {
	numFound := 0
	for _, nodeStatus := range *hostStatusMap {
		dynos, err := NodeStatusToDynos(&nodeStatus)
		if err != nil {
			return numFound, err
		}
		for _, dyno := range dynos {
			if dyno.Application == applicationName && dyno.Version == version {
				numFound += 1
			}
		}
	}
	return numFound, nil
}

func (this *Server) pruneDynos(nodeStatus NodeStatus, hostStatusMap *map[string]NodeStatus) error {
	cfg, err := this.getConfig(true)
	if err != nil {
		return err
	}

	logger := NewLogger(os.Stdout, "[dyno-cleanup] ")
	//fmt.Fprint(logger, "Pruning inactive dynos\n")

	type Key struct {
		application string
		version     string
		process     string
	}

	appMap := map[Key]bool{}

	appsByName := map[string]*Application{}

	// Build mapping of current expected app-process-versions.
	for _, app := range cfg.Applications {
		appsByName[app.Name] = app
		for process, _ := range app.Processes {
			//fmt.Fprintf(logger, "Existing app found, name=%v version=%v\n", app.Name, app.LastDeploy)
			appMap[Key{app.Name, app.LastDeploy, process}] = true
		}
	}

	e := &Executor{logger}

	dynos, err := NodeStatusToDynos(&nodeStatus)
	if err != nil {
		return err
	}

	// Cleanup running dynos which don't meet our criteria.
	for _, dyno := range dynos {
		destroy := false

		if dyno.State == DYNO_STATE_STOPPED {
			// Cleanup old stopped dynos which haven't already been reclaimed.
			app, ok := appsByName[dyno.Application]
			if ok {
				appVersionNumber, err := app.LastDeployNumber()
				if err != nil {
					// Not that bad
					fmt.Fprintf(logger, "error: failed to parse last deploy version number for app '%v'/'%v', ignoring..\n", app.Name, app.LastDeploy)
				}
				// If dyno is more than 5 revisions behind the latest, kill it.
				if dyno.VersionNumber+5 < appVersionNumber {
					fmt.Fprintf(logger, "stopped app container '%v' is more than 5 versions behind the latest, terminating it (latest version=%v)\n", dyno.Container, app.LastDeploy)
					destroy = true
				}
			} else {
				fmt.Fprintf(logger, "warning: unrecognized application '%v', ignoring..\n", dyno.Application)
			}

		} else if dyno.State == DYNO_STATE_RUNNING {
			key := Key{dyno.Application, dyno.Version, dyno.Process}
			_, ok := appMap[key]
			if !ok {
				// Verify that the app has some dynos running at the current version.
				app, ok := appsByName[dyno.Application]
				if ok {
					if app.TotalRequestedDynos() > 0 {
						numAtCurrentVersion, err := this.numDynosAtVersion(app.Name, app.LastDeploy, hostStatusMap)
						if err != nil {
							return err
						}
						if dyno.Version != app.LastDeploy && numAtCurrentVersion > 0 {
							fmt.Fprintf(logger, "app container '%v' looks like an old version, terminating it (%v dynos running at latest version=%v)\n", dyno.Container, numAtCurrentVersion, app.LastDeploy)
							destroy = true
						} else {
							fmt.Fprintf(logger, "app for container '%v' doesn't appear to have any dynos running at latest version=%v, refusing to take any action\n", dyno.Container, app.LastDeploy)
						}
					} else {
						fmt.Fprintf(logger, "app '%v' has no processes scaled up, terminating it\n", dyno.Application)
						destroy = true
					}
				} else {
					fmt.Fprintf(logger, "warning: unrecognized application '%v', ignoring..\n", dyno.Application)
				}

				if destroy {
					dynoInUseByLoadBalancer, err := this.dynoRoutingActive(&dyno)
					if err != nil {
						return err
					}
					if dynoInUseByLoadBalancer {
						fmt.Fprintf(logger, "app container '%v' is still in use by the current load-balancer configuration, termination cancelled\n", dyno.Container)
						destroy = false
					}
				}
			}
		}

		if destroy {
			// TODO: Add LB config check to ensure that dyno.Node + "-" + dyno.Port does not appear anywhere in the haproxy config.
			//"ssh", DEFAULT_NODE_USERNAME + "@" dyno.Host,
			fmt.Fprintf(logger, "Cleaning up trash name=%v version=%v\n", dyno.Application, dyno.Version)
			go func(dyno Dyno) {
				dyno.Shutdown(e)
			}(dyno)
		}
	}

	return nil
}

// Determine if a Dyno has active routes defined in the current load-balancer configuration.
func (this *Server) dynoRoutingActive(dyno *Dyno) (bool, error) {
	// LB config check to ensure that dyno.Node + "-" + dyno.Port does not appear anywhere in the haproxy config.
	// Non-web dynos have nothing to do with the load-balancer.
	if dyno.Process != "web" {
		return false, nil
	}

	config, err := this.getConfig(true)
	if err != nil {
		return true, err
	}

	// If there aren't any load-balancers, then the dyno certainly isn't being used by one.
	if len(config.LoadBalancers) == 0 {
		return false, nil
	}

	lbConfig, err := this.GetActiveLoadBalancerConfig()
	if err != nil {
		return true, err
	}

	expr := regexp.MustCompile(` backend ` + dyno.Application + ` ([^b]|b[^a]|ba[^c]|bac[^k]|back[^e]|backe[^n]|backen[^d])* ` + dyno.Host + `-` + dyno.Port)
	inUse := expr.MatchString(strings.Replace(lbConfig, "\n", " ", -1))
	return inUse, nil
}

// # Cleanup old versions on the shipbuilder build box.
// sudo lxc-ls --fancy | grep --only-matching '^[^ ]\+_v[0-9]\+ *STOPPED' | sed 's/^\([^ ]\+\)\(_v\)\([0-9]\+\) .*/\1 \3 \1\2\3/' | sort -t' ' -k 1,2 -g | awk -F ' ' '$1==app{ printf ",%s", $2 ; next } { app=$1 ; printf "\n%s %s", $1, $2 } END { printf "\n" }' | grep '^[^ ]\+ [0-9]\+,' | sed 's/,[0-9]\+$//' | awk -F ' ' '{ split($2,arr,",") ; for (i in arr) printf "%s_v%s\n", $1, arr[i] }' | xargs -n1 -IX bash -c 'attempts=0; rc=1; while [ $rc -ne 0 ] && [ $attempts -lt 10 ] ; do echo "rc=${rc}, attempts=${attempts} X"; sudo lxc-destroy -n X; rc=$?; attempts=$(($attempts + 1)); done'

// # Cleanup old zfs container volumes not in use.
// containers=$(sudo lxc-ls --fancy | sed "1,2d" | cut -f1 -d" ") ; for x in $(sudo zfs list | sed "1d" | cut -d" " -f1); do if [ "${x}" = "tank" ] || [ "${x}" = "tank/git" ] || [ "${x}" = "tank/lxc" ]; then echo "skipping bare tank, git, or lxc: ${x}"; continue; fi; if [ -n "$(echo $x | grep '@')" ]; then search=$(echo $x | sed "s/^.*@//"); else search=$(echo $x | sed "s/^[^\/]\{1,\}\///"); fi; if [ -z "$(echo -e "${containers}" | grep "${search}")" ]; then echo "destroying non-container zfs volume: $x" ; sudo zfs destroy $x; fi; done

// containers=$(sudo lxc-ls --fancy | sed "1,2d" | cut -f1 -d" ") ; for x in $(sudo zfs list | sed "1d" | cut -d" " -f1); do if [ "${x}" = "tank" ] || [ "${x}" = "tank/git" ] || [ "${x}" = "tank/lxc" ]; then echo "skipping bare tank, git, or lxc: ${x}"; continue; fi; if [ -n "$(echo $x | grep '@')" ]; then search=$(echo $x | sed "s/^.*@//"); else search=$(echo $x | sed "s/^[^\/]\{1,\}\///"); fi; if [ -z "$(echo -e "${containers}" | grep "${search}")" ]; then echo "destroying non-container zfs volume: $x" ; sudo zfs destroy $x; fi; done

// # Destroy stopped versioned containers (no base images).
// sudo lxc-ls --stopped | grep '_v[0-9]\+' | cut -f1 -d' ' | xargs -n1 sudo lxc-destroy -n

// # Cleanup empty container dirs.
// for dir in $(find /var/lib/lxc/ -maxdepth 1 -type d); do if test "${dir}" = '.' || test -z "$(echo "${dir}" | sed 's/\/var\/lib\/lxc\///')"; then continue; fi; count=$(find "${dir}/rootfs/" | head -n 3 | wc -l); if test $count -eq 1; then echo $dir $count; sudo rm -rf $dir; fi; done

// # Remove some list of containers.
// $ containers='a
// b
// c
// ..
// z'
// $ for c in $containers; do sudo lxc-stop -n $c; sudo lxc-destroy -n $c; sudo lxc-destroy -n $c; sudo lxc-destroy -n $c; done
