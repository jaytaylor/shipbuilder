package data

//go:generate bash -c "set -o errexit && set -o pipefail && ( command -v go-bindata || go get -u github.com/jteeuwen/go-bindata/... ) && echo 'Generating data package from go-bindata assets: ../../../build-packs/*' && cd ../../../build-packs && go-bindata -pkg='data' -o ../pkg/bindata_buildpacks/data/buildpacks_data.go * && cd - && gofmt -w buildpacks_data.go || ( echo 'oops.. there was a problem, you may need to install go-bindata, see README.md' && exit 1 )"
