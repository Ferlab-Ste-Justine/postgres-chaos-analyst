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

type DisruptionTarget int

const (
	Leader DisruptionTarget = iota
	SyncStandby
	Cluster
)

type DisruptionType int
const (
	Destruction DisruptionType = iota
	Reboot
)

func validateLosses(conf config.Config, disruptionTarget DisruptionTarget, disruptionType DisruptionType, log logger.Logger) {
	var table string
	var iterCount int64
	var action string
	var action2 string
	switch disruptionTarget {
	case Leader:
		switch disruptionType {
		case Destruction:
			table = "loss_leader_updater"
			iterCount = conf.Tests.LeaderLosses
			action = "leader loss"
			action2 = "rebuilding"
		case Reboot:
			table = "reboot_leader_updater"
			iterCount = conf.Tests.LeaderReboots
			action = "leader reboot"
			action2 = "restarting"
		}
	case SyncStandby:
		switch disruptionType {
		case Destruction:
			table = "loss_sync_standby_updater"
			iterCount = conf.Tests.SyncStanbyLosses
			action = "sync standby loss"
			action2 = "rebuilding"
		case Reboot:
			table = "reboot_sync_standby_updater"
			iterCount = conf.Tests.SyncStanbyReboots
			action = "sync standby reboot"
			action2 = "restarting"
		}
	case Cluster:
		switch disruptionType {
		case Destruction:
			return
		case Reboot:
			table = "reboot_cluster_updater"
			iterCount = conf.Tests.ClusterReboots
			action = "cluster reboot"
			action2 = "restarting"
		}
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
			switch disruptionTarget {
			case Leader:
				nodeName = clus.GetLeader().Name
			case SyncStandby:
				nodeName = clus.GetSyncStandby().Name
			case Cluster:
				nodeName = ""
			}

			beginning := time.Now()

			var terErr error
			switch disruptionType {
			case Destruction:
				terErr = terraform.SetServerStatus(nodeName, false, true, &conf.Terraform, log)
			case Reboot:
				terErr = terraform.SetServerStatus(nodeName, true, false, &conf.Terraform, log)
			}
			if terErr != nil {
				crResCh <- terErr
				return
			}

			switch disruptionType {
			case Destruction:
				if conf.Tests.RebuildPause.Nanoseconds() > 0 {
					if nodeName != "" {
						log.Infof("Pausing for %s before %s server \"%s\"", conf.Tests.RebuildPause.String(), action2, nodeName)
					} else {
						log.Infof("Pausing for %s before %s all servers", conf.Tests.RebuildPause.String(), action2)
					}
					time.Sleep(conf.Tests.RebuildPause)
				}
			case Reboot:
				if conf.Tests.RestartPause.Nanoseconds() > 0 {
					if nodeName != "" {
						log.Infof("Pausing for %s before %s server \"%s\"", conf.Tests.RestartPause.String(), action2, nodeName)
					} else {
						log.Infof("Pausing for %s before %s all servers", conf.Tests.RestartPause.String(), action2, nodeName)
					}
					time.Sleep(conf.Tests.RestartPause)
				}
			}

			terErr = terraform.SetServerStatus(nodeName, true, true, &conf.Terraform, log)
			if terErr != nil {
				crResCh <- terErr
				return
			}

			var healthErr error
			switch disruptionType {
			case Destruction:
				healthErr = pClient.WaitForHealthy(conf.Tests.LossRecoverTimeout, len(clus.Members))
			case Reboot:
				healthErr = pClient.WaitForHealthy(conf.Tests.RebootRecoverTimeout, len(clus.Members))
			}
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

	if conf.Tests.LeaderLosses > 0 {
		validateLosses(conf, Leader, Destruction, log)
	}

	if conf.Tests.SyncStanbyLosses > 0 {
		validateLosses(conf, SyncStandby, Destruction, log)
	}

	if conf.Tests.LeaderReboots > 0 {
		validateLosses(conf, Leader, Reboot, log)
	}

	if conf.Tests.SyncStanbyReboots > 0 {
		validateLosses(conf, SyncStandby, Reboot, log)
	}

	if conf.Tests.ClusterReboots > 0 {
		validateLosses(conf, Cluster, Reboot, log)
	}
}