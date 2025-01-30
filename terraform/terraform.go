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
	Up   bool
}

type ServersStatus struct {
	Cluster []ServerStatus
}

func (status *ServersStatus) SetActivation(name string, up bool) {
	for idx, _ := range status.Cluster {
		if status.Cluster[idx].Name == name {
			status.Cluster[idx].Up = up
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

func SetServerActivation(name string, up bool, conf *config.TerraformConfig, log logger.Logger) error {
	clusPath := path.Join(conf.Directory, conf.ClusterFile)
	
	status, readErr := readServerStatus(clusPath)
	if readErr != nil {
		return readErr
	}

	status.SetActivation(name, up)

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
	if up {
		action = "created"
	} else {
		action = "destroyed"
	}
	log.Infof("Server \"%s\" has been %s", name, action)

	return nil
}