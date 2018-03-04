package core

import (
	"bytes"
	"reflect"
	"testing"
	"time"
)

func TestDeployHookURLsDiscovery(t *testing.T) {
	testCases := []struct {
		env      map[string]string
		expected []string
	}{
		{
			env: map[string]string{
				"SB_DEPLOYHOOK_URL": "http://jaytaylor.com/v1/shipbuilder/deployed",
			},
			expected: []string{"http://jaytaylor.com/v1/shipbuilder/deployed"},
		},
		{
			env: map[string]string{
				"DEPLOYHOOKS_HTTP_URL": "http://jaytaylor.com/v1/shipbuilder/deployed",
			},
			expected: []string{"http://jaytaylor.com/v1/shipbuilder/deployed"},
		},
		{
			env: map[string]string{
				"SB_DEPLOYHOOK_URL_5": "http://jaytaylor.com/v1/shipbuilder/deployed",
			},
			expected: []string{"http://jaytaylor.com/v1/shipbuilder/deployed"},
		},
		{
			env: map[string]string{
				"DEPLOYHOOKS_HTTP_URL_8": "http://jaytaylor.com/v1/shipbuilder/deployed",
			},
			expected: []string{"http://jaytaylor.com/v1/shipbuilder/deployed"},
		},
		{
			env: map[string]string{
				"SB_DEPLOYHOOK_URL":    "http://jaytaylor.com/v1/0",
				"DEPLOYHOOKS_HTTP_URL": "http://jaytaylor.com/v1/1",
			},
			expected: []string{
				"http://jaytaylor.com/v1/0",
				"http://jaytaylor.com/v1/1",
			},
		},
		{
			env: map[string]string{
				"SB_DEPLOYHOOK_URL":      "http://jaytaylor.com/v1/0",
				"DEPLOYHOOKS_HTTP_URL_0": "http://jaytaylor.com/v1/1",
			},
			expected: []string{
				"http://jaytaylor.com/v1/0",
				"http://jaytaylor.com/v1/1",
			},
		},
		{
			env: map[string]string{
				"SB_DEPLOYHOOK_URL":   "http://jaytaylor.com/v1/0",
				"SB_DEPLOYHOOK_URL_1": "http://jaytaylor.com/v1/1",
			},
			expected: []string{"http://jaytaylor.com/v1/0", "http://jaytaylor.com/v1/1"},
		},
		{
			env: map[string]string{
				"SB_DEPLOYHOOK_URL":   "http://jaytaylor.com/v1/",
				"SB_DEPLOYHOOK_URL_0": "http://jaytaylor.com/v1/0",
				"SB_DEPLOYHOOK_URL_1": "http://jaytaylor.com/v1/1",
				"SB_DEPLOYHOOK_URL_2": "http://jaytaylor.com/v1/2",
			},
			expected: []string{
				"http://jaytaylor.com/v1/",
				"http://jaytaylor.com/v1/0",
				"http://jaytaylor.com/v1/1",
				"http://jaytaylor.com/v1/2",
			},
		},
		{
			env: map[string]string{
				"SB_DEPLOYHOOK_URL":    "http://jaytaylor.com/v1/",
				"SB_DEPLOYHOOK_URL_0":  "http://jaytaylor.com/v1/0",
				"SB_DEPLOYHOOK_URL_1":  "http://jaytaylor.com/v1/1",
				"SB_DEPLOYHOOK_URL_2":  "http://jaytaylor.com/v1/2",
				"SB_DEPLOYHOOK_URL_11": "http://jaytaylor.com/v1/11",
			},
			expected: []string{
				"http://jaytaylor.com/v1/",
				"http://jaytaylor.com/v1/0",
				"http://jaytaylor.com/v1/1",
				"http://jaytaylor.com/v1/11",
				"http://jaytaylor.com/v1/2",
			},
		},
	}

	for i, testCase := range testCases {
		var (
			server = &Server{
				ListenAddr:          "127.0.0.1:0",
				LogServerListenAddr: "127.0.0.1:0",
				Name:                "Testing",
				ImageURL:            "",
				ConfigFile:          "/tmp/sb-test.json",
			}
			app = &Application{
				Name:        "simplepie",
				BuildPack:   "python",
				Domains:     []string{"goggle.com"},
				Drains:      []string{},
				Environment: testCase.env,
				Processes: map[string]int{
					"web": 1,
				},
			}
			buf = &bytes.Buffer{}
			d   = &Deployment{
				Application: app,
				Logger:      buf,
				Revision:    "cafebabe",
				ScalingOnly: false,
				Server:      server,
				StartedTs:   time.Now(),
				Version:     "v1",
			}
		)

		actual := d.deployHookURLs()
		if !reflect.DeepEqual(actual, testCase.expected) {
			t.Errorf("[i=%v] Test case failed\nExpected deploy-hook URLs (len=%v):\n\t%v\nbut actual result was (len=%v):\n\t%v", i, len(testCase.expected), testCase.expected, len(actual), actual)
		}
	}
}
