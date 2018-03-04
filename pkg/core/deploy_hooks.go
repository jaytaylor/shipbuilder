package core

// All things related to deployment hook callbacks.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gigawattio/concurrency"
	log "github.com/sirupsen/logrus"
)

// DeployHookFunc is the interface deployment hook functions adhere to.
type DeployHookFunc func(d *Deployment, hookURL string, message string, alert bool) error

func (d *Deployment) postDeployHooks(deployErr error) {
	var (
		message                  string
		alert                    bool
		revision                 = "."
		durationFractionStripper = regexp.MustCompile(`^(.*)\.[0-9]*(s)?$`)
		duration                 = durationFractionStripper.ReplaceAllString(time.Since(d.StartedTs).String(), "$1$2")
		hookURLs                 = d.deployHookURLs()
	)

	if len(hookURLs) == 0 {
		log.Info("App %q doesn't have a SB_DEPLOYHOOKS_HTTP_URL set", d.Application.Name)
		return
	}

	if len(d.Revision) > 0 {
		revision = " (" + d.Revision[0:7] + ")."
	}

	if deployErr != nil {
		task := "Deployment"
		if d.ScalingOnly {
			task = "Scaling"
		}
		message = d.Application.Name + ": " + task + " operation failed after " + duration + ": " + deployErr.Error() + revision
		alert = true
	} else if deployErr == nil && d.ScalingOnly {
		procInfo := ""
		err := d.Server.WithApplication(d.Application.Name, func(app *Application, cfg *Config) error {
			for proc, val := range app.Processes {
				procInfo += " " + proc + "=" + strconv.Itoa(val)
			}
			return nil
		})
		if err != nil {
			log.Warnf("PostDeployHooks scaling caught: %v (continuing on..)", err)
		}
		if len(procInfo) > 0 {
			message = "Scaled " + d.Application.Name + " to" + procInfo + " in " + duration + revision
		} else {
			message = "Scaled down all " + d.Application.Name + " processes down to 0"
		}
	} else {
		message = "Deployed " + d.Application.Name + " " + d.Version + " in " + duration + revision
	}

	deployHookFuncs := []func() error{}

	for _, hookURL := range hookURLs {
		func(hookURL string) {
			for prefix, fn := range d.Server.deployHooksMap {
				if regexp.MustCompile(prefix).MatchString(hookURL) {
					simpleFunc := func() error {
						log.Infof("Dispatching deploy-hook callback for app=%v prefix=%v", d.Application.Name, prefix)
						return fn(d, hookURL, message, alert)
					}
					deployHookFuncs = append(deployHookFuncs, simpleFunc)
				}
			}
		}(hookURL)
	}

	if err := concurrency.MultiGo(deployHookFuncs...); err != nil {
		log.Errorf("Problem running deploy-hook callbacks: %s", err)
	}
}

func (d *Deployment) deployHookURLs() []string {
	var (
		envVarKeys = []string{ // Be forgiving and support legacy / deprecated environment variable keys.
			"SB_DEPLOYHOOKS_HTTP_URL",
			"SB_DEPLOYHOOKS_HTTP_URLS",
			"DEPLOYHOOKS_HTTP_URL",
			"DEPLOYHOOKS_HTTP_URLS",
		}
		hookURLs string
		ok       bool
	)
	for _, key := range envVarKeys {
		hookURLs, ok = d.Application.Environment[key]
		if ok {
			return strings.Split(hookURLs, ",")
		}
	}
	return nil
}

func (_ *Server) defaultDeployHooks() map[string]DeployHookFunc {
	// deployHooksMap follows the form of regExpPrefix->callbackHandler.
	deployHooksMap := map[string]DeployHookFunc{
		// HipChat.
		"^https://api.hipchat.com/v1/rooms/message.*": func(d *Deployment, hookURL string, message string, alert bool) error {
			var (
				notify = 0
				color  = "green"
			)
			if alert {
				notify = 1
				color = "red"
			}
			hookURL = fmt.Sprintf("%v&notify=%v&color=%v&from=%v&message_format=text&message=%v", hookURL, notify, color, d.Server.Name, url.QueryEscape(message))
			response, err := http.Get(hookURL)
			if err != nil {
				return fmt.Errorf("hipchat deploy-hook: %s", err)
			}
			if response.StatusCode/100 != 2 {
				return fmt.Errorf("hipchat deploy-hook: received non-200 status code=%v", response.StatusCode)
			}
			return nil
		},

		// Slack.
		"^https://hooks.slack.com/services/.*": func(d *Deployment, hookURL string, message string, alert bool) error {
			data := map[string]interface{}{
				"text":         message,
				"username":     d.Server.Name,
				"icon_url":     d.Server.ImageURL,
				"unfurl_links": false,
				"unfurl_media": false,
				// TODO: Attach deployment log with message.
			}
			if alert {
				data["text"] = fmt.Sprintf("!here %v", data["text"])
			}
			payload, err := json.Marshal(data)
			if err != nil {
				return fmt.Errorf("slack deploy-hook: marshalling JSON: %s", err)
			}
			response, err := http.Post(hookURL, "application/json", bytes.NewBuffer(payload))
			if err != nil {
				return fmt.Errorf("slack deploy-hook: %s", err)
			}
			if response.StatusCode/100 != 2 {
				return fmt.Errorf("slack deploy-hook: received non-200 status code=%v", response.StatusCode)
			}
			return nil
		},

		// New-Relic.
		"^https://api.newrelic.com/v2/applications/[^/]+/deployments.json": func(d *Deployment, hookURL string, message string, alert bool) error {
			apiKey, ok := d.Application.Environment["SB_NEWRELIC_API_KEY"]
			if !ok {
				return fmt.Errorf("new-relic deploy-hook: missing app environment variable %q", "SB_NEWRELIC_API_KEY")
			}

			data := map[string]map[string]interface{}{
				"deployment": map[string]interface{}{
					"revision":    d.Revision[0:7],
					"changelog":   message,
					"description": fmt.Sprintf("version=%v", d.Version),
					"user":        "Anomali", // TODO: Determine deploying user.
				},
			}

			payload, err := json.Marshal(data)
			if err != nil {
				return fmt.Errorf("new-relic deploy-hook: marshalling JSON: %s", err)
			}
			request, err := http.NewRequest("POST", hookURL, bytes.NewReader(payload))
			if err != nil {
				return fmt.Errorf("new-relic deploy-hook: constructing request: %s", err)
			}

			request.Header.Add("Content-Type", "application/json")
			request.Header.Add("X-Api-Key", apiKey)

			client := &http.Client{}
			response, err := client.Do(request)
			if err != nil {
				return fmt.Errorf("new-relic deploy-hook: %s", err)
			}
			if response.StatusCode/100 != 2 {
				return fmt.Errorf("new-relic deploy-hook: received non-200 status code=%v", response.StatusCode)
			}
			return nil
		},
	}

	return deployHooksMap
}
