package main

import (
	"fmt"
	"time"

	"github.com/Ferlab-Ste-Justine/postgres-chaos-analyst/config"
	"github.com/Ferlab-Ste-Justine/postgres-chaos-analyst/logger"
	"github.com/Ferlab-Ste-Justine/postgres-chaos-analyst/measure"
	"github.com/Ferlab-Ste-Justine/postgres-chaos-analyst/patroni"
	"github.com/Ferlab-Ste-Justine/postgres-chaos-analyst/terraform"
)

func validateSwitchovers(conf config.Config, log logger.Logger) {
	doneCh := make(chan struct{})
	measResCh := measure.Measure(
		&measure.Updater{
			TableName: "switchover_updater",
		},
		&conf.PgClient,
		doneCh,
		log,
	)

	swResCh := make(chan error)

	go func() {
		defer func() {
			select {
			case <- doneCh:
			default:
				close(doneCh)
			}
		}()
		
		pClient, pClientErr := patroni.NewPatroniClient(&conf.PatroniClient, log)
		if pClientErr != nil {
			swResCh <- pClientErr
			return
		}

		iterations := int64(0)
		for iterations < conf.Tests.Switchovers {
			changeErr := pClient.ForceLeaderChange(conf.Tests.ChangeRecoverTimeout)
			if changeErr != nil {
				swResCh <- changeErr
				return
			}

			time.Sleep(conf.Tests.ValidationInterval)
			iterations += 1
		}

		close(swResCh)
	}()
	
	swErr := <- swResCh
	measRes := <- measResCh

	AbortOnErr("Error occurred while overseeing the patroni leadership switchovers: %s", swErr)
	AbortOnErr("Error occurred while running transactions on postgres cluster: %s", measRes.Error)

	log.Infof("Diagnostics running %d patroni switchovers with %s rest interval in between:\n%s", conf.Tests.Switchovers, conf.Tests.ValidationInterval.String(), measRes.Measurements.String())
}

type CrashTarget int

const (
	Leader CrashTarget = iota
	SyncStandby
)

func validateCrashes(conf config.Config, crashTarget CrashTarget, log logger.Logger) {
	var table string
	var iterCount int64
	var action string
	switch crashTarget {
	case Leader:
		table = "crash_leader_updater"
		iterCount = conf.Tests.LeaderCrashes
		action = "leader crashes"
	case SyncStandby:
		table = "crash_sync_standby_updater"
		iterCount = conf.Tests.SyncStanbyCrashes
		action = "sync standby crashes"
	}
	
	doneCh := make(chan struct{})
	measResCh := measure.Measure(
		&measure.Updater{
			TableName: table,
		},
		&conf.PgClient,
		doneCh,
		log,
	)

	crResCh := make(chan error)

	go func() {
		defer func() {
			select {
			case <- doneCh:
			default:
				close(doneCh)
			}
		}()
		
		pClient, pClientErr := patroni.NewPatroniClient(&conf.PatroniClient, log)
		if pClientErr != nil {
			crResCh <- pClientErr
			return
		}

		iterations := int64(0)
		for iterations < iterCount {
			clus, clusErr := pClient.GetCluster()
			if clusErr != nil {
				crResCh <- clusErr
				return
			}

			var nodeName string
			switch crashTarget {
			case Leader:
				nodeName = clus.GetLeader().Name
			case SyncStandby:
				nodeName = clus.GetSyncStandby().Name
			}

			beginning := time.Now()

			terErr := terraform.SetServerActivation(nodeName, false, &conf.Terraform, log)
			if terErr != nil {
				crResCh <- terErr
				return
			}

			terErr = terraform.SetServerActivation(nodeName, true, &conf.Terraform, log)
			if terErr != nil {
				crResCh <- terErr
				return
			}

			healthErr := pClient.WaitForHealthy(conf.Tests.CrashRecoverTimeout, len(clus.Members))
			if healthErr != nil {
				crResCh <- healthErr
				return
			}

			log.Infof("Fully recovered from %s to healthy cluster in %s", action, time.Now().Sub(beginning).String())

			time.Sleep(conf.Tests.ValidationInterval)
			iterations += 1
		}

		close(crResCh)
	}()
	
	crErr := <- crResCh
	measRes := <- measResCh

	AbortOnErr(fmt.Sprintf("Error occurred while overseeing the %s: %s", action, "%s"), crErr)
	AbortOnErr("Error occurred while running transactions on postgres cluster: %s", measRes.Error)

	log.Infof("Diagnostics running %d %s with %s rest interval in between:\n%s", iterCount, action, conf.Tests.ValidationInterval.String(), measRes.Measurements.String())
}

func main() {
	conf, confErr := config.GetConfig(getEnv("PG_CHAOS_ANALYST_CONFIG_FILE", "config.yml"))
	AbortOnErr("Error getting configurations: %s", confErr)

	log := logger.Logger{LogLevel: conf.GetLogLevel()}

	if conf.Tests.Switchovers > 0 {
		validateSwitchovers(conf, log)
	}

	if conf.Tests.LeaderCrashes > 0 {
		validateCrashes(conf, Leader, log)
	}

	if conf.Tests.SyncStanbyCrashes > 0 {
		validateCrashes(conf, SyncStandby, log)
	}

}