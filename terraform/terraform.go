package terraform

import (
	"context"
	"io/ioutil"
	"os"
	"os/exec"
	"path"

	"github.com/Ferlab-Ste-Justine/postgres-chaos-analyst/config"
	"github.com/Ferlab-Ste-Justine/postgres-chaos-analyst/logger"

	"github.com/hashicorp/terraform-exec/tfexec"
	yaml "gopkg.in/yaml.v2"
)

type ServerStatus struct {
	Name string
	Exists bool
	Running bool
}

type ServersStatus struct {
	Cluster []ServerStatus
}

func (status *ServersStatus) SetStatus(name string, exists bool, running bool) {
	for idx, _ := range status.Cluster {
		if status.Cluster[idx].Name == name || name == "" {
			status.Cluster[idx].Exists = exists
			status.Cluster[idx].Running = running
			break
		}
	}
}

func readServerStatus(fPath string) (ServersStatus, error) {
	var status ServersStatus

	data, err := ioutil.ReadFile(fPath)
	if err != nil {
		return status, err
	}

	err = yaml.Unmarshal(data, &status)
	return status, err
}

func persistServersStatus(fPath string, status ServersStatus) error {
	data, err := yaml.Marshal(&status)
	if err != nil {
		return err
	}

	return os.WriteFile(fPath, data, 0644)
}

func SetServerStatus(name string, exists bool, running bool, conf *config.TerraformConfig, log logger.Logger) error {
	clusPath := path.Join(conf.Directory, conf.ClusterFile)
	
	status, readErr := readServerStatus(clusPath)
	if readErr != nil {
		return readErr
	}

	status.SetStatus(name, exists, running)

	perErr := persistServersStatus(clusPath, status)
	if perErr != nil {
		return perErr
	}

	terPath, terErr := exec.LookPath("terraform")
	if terErr != nil {
		return terErr
	}

	tf, tfErr := tfexec.NewTerraform(conf.Directory, terPath)
	if tfErr != nil {
		return tfErr
	}

	initErr := tf.Init(context.Background(), tfexec.Upgrade(true))
	if initErr != nil {
		return initErr
	}

	applyErr := tf.Apply(context.Background())
	if applyErr != nil {
		return applyErr
	}

	var action string
	if exists && running {
		action = "set to exist and run"
	} else if !exists {
		action = "destroyed"
	} else {
		action = "stopped"
	}

	if name != "" {
		log.Infof("Server \"%s\" has been %s", name, action)
	} else {
		log.Infof("All servers has been %s", action)
	}

	return nil
}