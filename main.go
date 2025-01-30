package main

import (
	"time"

	"github.com/Ferlab-Ste-Justine/postgres-chaos-analyst/config"
	"github.com/Ferlab-Ste-Justine/postgres-chaos-analyst/logger"
	"github.com/Ferlab-Ste-Justine/postgres-chaos-analyst/measure"
	"github.com/Ferlab-Ste-Justine/postgres-chaos-analyst/patroni"
)

func main() {
	conf, confErr := config.GetConfig(getEnv("PG_CHAOS_ANALYST_CONFIG_FILE", "config.yml"))
	AbortOnErr("Error getting configurations: %s", confErr)

	log := logger.Logger{LogLevel: conf.GetLogLevel()}

	doneCh := make(chan struct{})
	measResCh := measure.Measure(
		&measure.Updater{
			TableName: "switchover_updater",
		},
		&conf.PgClient,
		conf.Tests.ConsFailTolerance,
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