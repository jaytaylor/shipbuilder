package core

import (
	"testing"

	"github.com/jaytaylor/shipbuilder/pkg/bindata_buildpacks"
	"github.com/jaytaylor/shipbuilder/pkg/releases"
)

func TestDynoPortAllocation(t *testing.T) {
	server := &Server{
		ListenAddr:          ":",
		LogServerListenAddr: ":59595",
		BuildpacksProvider:  bindata_buildpacks.NewProvider(),
		ReleasesProvider:    releases.NewFSReleasesProvider("/tmp/sb-dyno-test"),
		ConfigFile:          "/tmp/sb-test.json",
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
}

func TestContainerToDyno(t *testing.T) {
	testCases := []struct {
		host      string
		container string
		expectOK  bool
	}{
		{
			host:      "localhost",
			container: "devsb3-optic2-v1-web-10188-Running",
			expectOK:  true,
		},
		{
			host:      "localhost",
			container: "devsb3-optic2-v1-alerts-scheduledTasks-10999-Stopped",
			expectOK:  false,
		},
		{
			host:      "localhost",
			container: "devsb3-optic2-v1-alertsScheduledTasks-10999-Stopped",
			expectOK:  true,
		},
	}

	for i, testCase := range testCases {
		dyno, err := ContainerToDyno(testCase.host, testCase.container)
		if testCase.expectOK && err != nil {
			t.Fatalf("[i=%v] Expected err=nil but err=%v", i, err)
		} else if !testCase.expectOK && err == nil {
			t.Fatalf("[i=%v] Expected test case to fail, but err=%v", i, err)
		}
		t.Logf("dyno=%# v", dyno)
	}
}
