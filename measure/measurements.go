package measure

import (
	"fmt"
	"strings"
	"time"

	"github.com/Ferlab-Ste-Justine/postgres-chaos-analyst/config"
	"github.com/Ferlab-Ste-Justine/postgres-chaos-analyst/logger"
)

type Outages struct {
	Count         int64
	TotalDuration time.Duration
	Longest       time.Duration
}

type Measurements struct {
	TotalOps int64
	LostOps  int64
	GhostOps int64
	Outages  Outages
}

func (meas *Measurements) String() string {
	return strings.Join([]string{
		fmt.Sprintf("Total Ops: %d", meas.TotalOps),
		fmt.Sprintf("Lost Ops: %d", meas.LostOps),
		fmt.Sprintf("Ghost Ops: %d", meas.GhostOps),
		fmt.Sprintf("Outages:"),
		fmt.Sprintf("\tCount: %d", meas.Outages.Count),
		fmt.Sprintf("\tCumulative Duration: %s", meas.Outages.TotalDuration.String()),
		fmt.Sprintf("\tLongest One: %s", meas.Outages.Longest.String()),
	}, "\n")
}

type Anomaly int

const (
	NoProblem Anomaly = iota
	LostTransaction
	GhostTransaction
)

type MeasureResult struct {
	Measurements Measurements
	Error        error
}

type Tester interface {
	Initialize(*config.PgClientConfig) error
	Run(*config.PgClientConfig) (Anomaly, error)
	Cleanup(*config.PgClientConfig) error
	Id() string
}

func Measure(tester Tester, pgConf *config.PgClientConfig, done <-chan struct{}, log logger.Logger) <-chan MeasureResult {
	chRes := make(chan MeasureResult)

	go func() {
		initErr := tester.Initialize(pgConf)
		if initErr != nil {
			chRes <- MeasureResult{Error: initErr}
			return
		}

		var measurements Measurements

		var outageSince *time.Time
		for true {
			select {
			case <-done:
				cleanupErr := tester.Cleanup(pgConf)
				if cleanupErr != nil {
					log.Warnf("Test cleanup failed for tester \"%s\"", tester.Id())
				}

				chRes <- MeasureResult{Measurements: measurements, Error: nil}
				return
			default:
			}

			anomaly, runErr := tester.Run(pgConf)
			
			measurements.TotalOps += 1
			switch anomaly {
			case LostTransaction:
				measurements.LostOps += 1
				log.Infof("Tester \"%s\" lost a committed transaction", tester.Id())
			case GhostTransaction:
				measurements.GhostOps += 1
				log.Infof("Tester \"%s\" successfully committed transaction that was marked a failure", tester.Id())
			}

			if runErr != nil {
				if outageSince == nil {
					log.Infof("Tester \"%s\" outage started with error: %s", tester.Id(), runErr.Error())
					now := time.Now()
					outageSince = &now
					measurements.Outages.Count += 1
				} 
			} else {
				if outageSince != nil {
					outageDuration := time.Since(*outageSince)
					outageSince = nil
					if outageDuration.Nanoseconds() > measurements.Outages.Longest.Nanoseconds() {
						measurements.Outages.Longest = outageDuration
					}

					measurements.Outages.TotalDuration = time.Duration(measurements.Outages.TotalDuration.Nanoseconds() + outageDuration.Nanoseconds())
					log.Infof("Tester \"%s\" noticed a postgres outage for %s", tester.Id(), outageDuration.String())
				}
			}
		}
	}()

	return chRes
}