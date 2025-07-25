package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
	"time"
	"net/url"

	"github.com/Ferlab-Ste-Justine/postgres-chaos-analyst/logger"

	yaml "gopkg.in/yaml.v2"
)

type PasswordAuth struct {
	Username string
	Password string
}

type PgClientAuthConfig struct {
	CaCert       string `yaml:"ca_cert"`
	PasswordAuth string `yaml:"password_auth"`
	Username     string `yaml:"-"`
	Password     string `yaml:"-"`
}

type PgClientConfig struct {
	Endpoint          string
	Auth              PgClientAuthConfig
	Database          string
	ConnectionTimeout time.Duration      `yaml:"connection_timeout"`
	QueryTimeout      time.Duration      `yaml:"query_timeout"`
}

func (conf *PgClientConfig) GetConnStr() string {
	conn := fmt.Sprintf("postgres://%s:%s@%s/%s", url.QueryEscape(conf.Auth.Username), url.QueryEscape(conf.Auth.Password), conf.Endpoint, conf.Database)
	if conf.Auth.CaCert != "" {
		conn = fmt.Sprintf("%s?sslmode=verify-full&sslrootcert=%s", conn, url.QueryEscape(conf.Auth.CaCert))
	}

	return conn
}

type CertAuth struct {
	CaCert     string `yaml:"ca_cert"`
	ClientCert string `yaml:"client_cert"`
	ClientKey  string `yaml:"client_key"`
}

type PatroniClientConfig struct {
	Endpoint          string
	Auth              CertAuth
	ConnectionTimeout time.Duration `yaml:"connection_timeout"`
	RequestTimeout    time.Duration `yaml:"request_timeout"`
}

type TestsConfig struct {
	Switchovers          int64
	LeaderLosses         int64         `yaml:"leader_losses"`
	SyncStanbyLosses     int64         `yaml:"sync_standby_losses"`
	LeaderReboots        int64         `yaml:"leader_reboots"`
	SyncStanbyReboots    int64         `yaml:"sync_standby_reboots"`
	ClusterReboots       int64         `yaml:"cluster_reboots"`
	ValidationInterval   time.Duration `yaml:"validation_interval"`
	ChangeRecoverTimeout time.Duration `yaml:"change_recover_timeout"`
	LossRecoverTimeout   time.Duration `yaml:"loss_recover_timeout"`
	RebootRecoverTimeout time.Duration `yaml:"reboot_recover_timeout"`
	RebuildPause         time.Duration `yaml:"rebuild_pause"`
	RestartPause         time.Duration `yaml:"restart_pause"`
}

type TerraformConfig struct {
	Directory   string
	ClusterFile string `yaml:"cluster_file"`
}

type Config struct {
	PgClient      PgClientConfig      `yaml:"postgres_client"`
	PatroniClient PatroniClientConfig `yaml:"patroni_client"`
	LogLevel      string              `yaml:"log_level"`
	Tests         TestsConfig
	Terraform     TerraformConfig
}

func (c *Config) GetLogLevel() int64 {
	logLevel := strings.ToLower(c.LogLevel)
	switch logLevel {
	case "error":
		return logger.ERROR
	case "warning":
		return logger.WARN
	case "debug":
		return logger.DEBUG
	default:
		return logger.INFO
	}
}

func GetPasswordAuth(path string) (PasswordAuth, error) {
	var a PasswordAuth

	b, err := ioutil.ReadFile(path)
	if err != nil {
		return a, errors.New(fmt.Sprintf("Error reading the etcd password auth file at path '%s': %s", path, err.Error()))
	}

	err = yaml.Unmarshal(b, &a)
	if err != nil {
		return a, errors.New(fmt.Sprintf("Error parsing the password auth file: %s", err.Error()))
	}

	return a, nil
}

func GetConfig(path string) (Config, error) {
	var c Config

	b, err := ioutil.ReadFile(path)
	if err != nil {
		return c, errors.New(fmt.Sprintf("Error reading the configuration file: %s", err.Error()))
	}

	err = yaml.Unmarshal(b, &c)
	if err != nil {
		return c, errors.New(fmt.Sprintf("Error parsing the configuration file: %s", err.Error()))
	}

	pAuth, pAuthErr := GetPasswordAuth(c.PgClient.Auth.PasswordAuth)
	if pAuthErr != nil {
		return c, pAuthErr
	}
	c.PgClient.Auth.Username = pAuth.Username
	c.PgClient.Auth.Password = pAuth.Password

	return c, nil
}
