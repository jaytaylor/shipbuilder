package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	COMPRESSED_PATH      = "/tmp/build.tar.gz"
	DEPLOYER_SCRIPT_PATH = "/tmp/deployer.sh"
)

var (
	defaultSshHost string
	defaultSshKey  string
	sshHost        = ConfigFromEnv("SB_SSH_HOST", getDefaultSshHost())
	sshKey         = ConfigFromEnv("SB_SSH_KEY", getDefaultSshKey())
	LDFLAGS_MAP    = map[string]string{
		"SB_SSH_HOST":            "main.defaultSshHost",
		"SB_SSH_KEY":             "main.defaultSshKey",
		"SB_AWS_KEY":             "main.defaultAwsKey",
		"SB_AWS_SECRET":          "main.defaultAwsSecret",
		"SB_AWS_REGION":          "main.defaultAwsRegion",
		"SB_S3_BUCKET":           "main.defaultS3BucketName",
		"SB_HAPROXY_CREDENTIALS": "main.defaultHaProxyCredentials",
		"SB_HAPROXY_STATS":       "main.defaultHaProxyStats",
		"LXC_FS":                 "main.defaultLxcFs",
		"ZFS_POOL":               "main.defaultZfsPool",
	}
	deployerScriptContent = `#!/usr/bin/env bash

################################################################################
# AUTO-GENERATED SHIPBUILDER SYSTEM DEPLOYMENT SCRIPT                          #
#                                                                              #
#   .                             .                                            #
#  /!\  NEVER RUN THIS MANUALLY  /!\                                           #
# '---'                         '---'                                          #
################################################################################

sudo -n echo 'testing for sudo capability' 1>/dev/null 2>/dev/null
rc=$?
test $rc -ne 0 && echo "error: sudo capability is required but not is not available for account=$(whoami)@$(hostname), operation aborted" 1>&2 && exit 1

set -e -x

rm -rf /tmp/build
mkdir -p /tmp/build
echo 'Extracting'
tar -C /tmp/build -xzf '` + COMPRESSED_PATH + `'

cd /tmp/build/src

export GOPATH=$HOME/go

set +e +x

#################################################################################################
#################################################################################################
##                                                                                             ##
##   TODO: This is over-complicated, see https://golang.org/cmd/go/ esp. re: go build -i ...   ##
##         or perhaps go list -f '{{join .Deps "\n"}}' | \                                     ##
##                        xargs go list -f '{{if not .Standard}}{{.ImportPath}}{{end}}'        ##
##                                                                                             ##
#################################################################################################
#################################################################################################

echo 'info: fetching dependencies'
# This finds all lines between:
# import (
#     ...
# )
# and appropriately filters the list down to the projects dependencies.
dependencies=$(
    find . -wholename '*.go' -exec awk '{ if ($1 ~ /^import/ && $2 ~ /[(]/) { s=1; next; } if ($1 ~ /[)]/) { s=0; } if (s) print; }' {} \; |
        grep -v '^[^\.]*$' |
        tr -d '\t' |
        tr -d '"' |
        sed 's/^\. \{1,\}//g' |
        sort |
        uniq |
        grep -v '^[ \t]*\/\/' |
        sed 's/_ //g'
)
for dependency in $dependencies; do
    echo "    retrieving: ${dependency}"
    if ! test -d "${GOPATH}/src/${dependency}"; then
        go get -u $dependency
        rc=$?

        echo "        -> already exists, skipping"
        rc=0
    fi
    test $rc -ne 0 && echo "error: retrieving dependency ${dependency} exited with non-zero status code ${rc}" && exit $rc
done

echo 'info: compiling shipbuilder daemon'
export target=/mnt/build/shipbuilder
sudo rm -f "${target}.new"
echo sudo -E go build ` + getLdFlags() + ` -a -x -o "${target}.new"
sudo -E go build ` + getLdFlags() + ` -a -o "${target}.new"
exitCode=$?
test $exitCode -ne 0 && echo "error: build step failed with exit status code=${exitCode}, operation aborted" 1>&2 && exit $exitCode

if [ -f "${target}.new" ]; then
    echo "info: shipbuilder successfully compiled, installing to ${target}"
    sudo mv ${target}{.new,}

    echo 'info: updating upstart script'
    cat <<EOF | sudo tee /etc/init/shipbuilder.conf >/dev/null
start on (local-filesystems and net-device-up IFACE!=lo)
stop on [!12345]

env GOTRACEBACK=crash
limit core unlimited unlimited

exec start-stop-daemon --start --chdir /mnt/build --exec /usr/bin/envdir /mnt/build/env /mnt/build/shipbuilder server 2>&1 | logger -t shipbuilder
EOF

    echo 'info: copying build-packs'
    sudo rm -rf /mnt/build/build-packs
    sudo mv /{tmp,mnt}/build/build-packs

    echo 'info: stopping service'
    sudo service shipbuilder stop

    echo 'info: starting service'
    sudo service shipbuilder start

    exitCode=0

else
    echo 'error: build failed; operation aborted'
    exitCode=1
fi

echo 'info: cleaning up build artifacts'
cd /tmp/build && rm -rf sb-prod sb-test whitelist.sh t.py* test *sublime* b.go env-prod env-testlab t.go && cd -
#sudo rm -f '` + COMPRESSED_PATH + `' '` + DEPLOYER_SCRIPT_PATH + `' '/tmp/build'
#echo "info: exiting status code=${exitCode}"
exit $exitCode`
)

func getLdFlags() string {
	// Require that an env/ dir exists.
	exists, err := PathExists("env")
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	if !exists {
		fmt.Printf("error: 'env' configuration directory missing, create it to continue (see README for more information)\n")
		os.Exit(1)
	}
	ldflags := ""
	err = filepath.Walk("env/", func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			key := strings.Split(path, "/")[1]
			flagName, ok := LDFLAGS_MAP[key]
			if ok {
				if len(ldflags) == 0 {
					ldflags = "-ldflags '"
				} else {
					ldflags += " "
				}
				data, err := ioutil.ReadFile(path)
				if err != nil {
					return err
				}
				// Only use the value from the first line of the file.
				value := strings.TrimSpace(strings.Split(string(data), "\n")[0])
				ldflags += "-X " + flagName + " " + value
			}
			return nil
		}
		return nil
	})
	if len(ldflags) > 0 {
		ldflags += "'"
	}
	//fmt.Printf("/%v/\n", ldflags)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	return ldflags
}

func getDefaultSshHost() string {
	if defaultSshHost != "" {
		return defaultSshHost
	}
	return "ubuntu@pushit.sendhub.com"
}

func getDefaultSshKey() string {
	if defaultSshKey != "" {
		return defaultSshKey
	}
	return os.Getenv("HOME") + "/.ssh/pk-jay.pem"
}

func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func ConfigFromEnv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		value = defaultValue
	}
	return value
}

func run(c string, args ...string) error {
	fmt.Println(c, args)
	cmd := exec.Command(c, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func deploy() error {
	fmt.Printf("info: Deploying to target: %v\n", sshHost)
	os.Remove(DEPLOYER_SCRIPT_PATH)
	os.Remove(COMPRESSED_PATH)

	// Write out deployer script.
	ioutil.WriteFile(DEPLOYER_SCRIPT_PATH, []byte(deployerScriptContent), 0777)
	// If "-u|--update" flag is passed, transform deployer script to always update dependencies even when they are present.
	if len(os.Args) > 1 && (os.Args[1] == "-u" || os.Args[1] == "--update") {
		fmt.Printf("info: dependency updates will be forced\n")
		run("bash", "-c",
			`sed -i.bak 's/if ! test -d "\${GOPATH}\/src\/\${dependency}"; then go get -u \$dependency; rc=\$?; else echo "        -> already exists, skipping"; rc=0; fi/go get -u \${dependency}; rc=\$?/g' '`+DEPLOYER_SCRIPT_PATH+`'; rm -f '`+DEPLOYER_SCRIPT_PATH+`.bak'`,
		)
	}

	// Upload latest code + deployment shell script to the server.
	err := run("bash", "-c", `
echo 'compressing..'
tar --exclude ./shipbuilder --exclude ./.git -czf '`+COMPRESSED_PATH+`' .
echo 'uploading..'
chmod a+x '`+DEPLOYER_SCRIPT_PATH+`'
rsync -azve 'ssh -i "`+sshKey+`" -o "StrictHostKeyChecking=no" -o "BatchMode=yes"' '`+COMPRESSED_PATH+`' '`+DEPLOYER_SCRIPT_PATH+`' `+sshHost+`:/tmp/
ssh -i '`+sshKey+`' -o 'StrictHostKeyChecking=no' `+sshHost+` /bin/bash '`+DEPLOYER_SCRIPT_PATH+`'`)
	if err != nil {
		return err
	}
	/*fmt.Printf("go env = %v", os.Getenv("GOPATH"))
	args := []string{
		"build",
		"-o", os.Getenv("HOME") + "/Dropbox/SendHub\\ General/Engineering\\ Resources/bin/shipbuilder",
	}
	err = filepath.Walk("src", func(p string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return err
		}
		if strings.HasSuffix(p, ".go") {
			args = append(args, p)
		}
		return err
	})
	if err != nil {
		return err
	}
	// Build & copy to dropbox
	err = run("go", args...)*/
	return err
}

func main() {
	err := deploy()
	if err != nil {
		panic(err)
	}
}
