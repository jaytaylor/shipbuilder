package main

import (
	"fmt"
	"io"
	"os"

	"github.com/robfig/cron"
)

type (
	CronTask struct {
		Name     string
		Schedule string
		Fn       func(io.Writer) error
	}
)

func (this *Server) GetCronTasks() []CronTask {
	cronTasks := []CronTask{
		// ZFS maintenance.
		CronTask{
			Name:     "ZfsMaintenance",
			Schedule: "1 30 7 * * *",
			Fn:       this.sysPerformZfsMaintenance,
		},
		// Orphaned snapshots removal.
		CronTask{
			Name:     "OrphanedSnapshots",
			Schedule: "1 45 * * * *",
			Fn:       this.sysRemoveOrphanedReleaseSnapshots,
		},
		// Hourly NTP sync.
		CronTask{
			Name:     "NtpSync",
			Schedule: "1 1 * * * *",
			Fn:       this.sysSyncNtp,
		},
	}
	return cronTasks
}

func (this *Server) startCrons() {
	c := cron.New()
	fmt.Printf("[cron] Configuring..\n")
	for _, cronTask := range this.GetCronTasks() {
		if cronTask.Name == "ZfsMaintenance" && lxcFs != "zfs" {
			fmt.Printf(`[cron] Refusing to add ZFS maintenance cron task because the lxcFs is actuallty "%v"\n`, lxcFs)
			continue
		}
		fmt.Printf("[cron] Adding cron task '%v'\n", cronTask.Name)
		c.AddFunc(cronTask.Schedule, func() {
			logger := NewLogger(os.Stdout, "["+cronTask.Name+"] ")
			err := cronTask.Fn(logger)
			if err != nil {
				fmt.Printf("cron: %v ended with error=%v\n", cronTask.Name, err)
			}
		})
	}
	fmt.Printf("[cron] Starting..\n")
	c.Start()
	fmt.Printf("[cron] Cron successfully launched.\n")
}
