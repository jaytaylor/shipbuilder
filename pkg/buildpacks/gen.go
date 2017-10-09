package buildpacks

//go:generate bash -c "set -o errexit && set -o pipefail && ( command -v go-bindata || go get -u github.com/jteeuwen/go-bindata/... ) && echo 'Generating buildpacks package from go-bindata asset: ../../build-packs/*' && cd ../../build-packs && go-bindata -pkg='buildpacks' -o ../pkg/buildpacks/buildpacks.go * && cd - && gofmt -w buildpacks.go || ( echo 'oops.. there was a problem, you may need to install go-bindata, see README.md' && exit 1 )"
