package patroni

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/Ferlab-Ste-Justine/postgres-chaos-analyst/config"
	"github.com/Ferlab-Ste-Justine/postgres-chaos-analyst/logger"
)

var switchoverResponseRegex *regexp.Regexp

func init() {
	switchoverResponseRegex = regexp.MustCompile(`^Successfully switched over to "(?P<leader>.*)"$`)
}

type PatroniMemberLag int64
func (lag *PatroniMemberLag) UnmarshalJSON(data []byte) error {
    val, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		*lag = PatroniMemberLag(-1)
	} else {
		*lag = PatroniMemberLag(val)
	}

    return nil
}

type PatroniMember struct {
	Name     string           `json:"name"`
	Role     string           `json:"role"`
	State    string           `json:"state"`
	ApiUrl   string           `json:"api_url"`
	Host     string           `json:"host"`
	Port     int64            `json:"port"`
	Timeline int64            `json:"timeline"`
	Lag      PatroniMemberLag `json:"lag"`
}

type PatroniCluster struct {
	Members []PatroniMember `json:"members"`
	Scope   string          `json:"scope"`
}

func (cluster *PatroniCluster) GetLeader() PatroniMember {
	for _, member := range cluster.Members {
		if member.Role == "leader" {
			return member
		}
	}

	return PatroniMember{}
}

func (cluster *PatroniCluster) GetSyncStandby() PatroniMember {
	for _, member := range cluster.Members {
		if member.Role == "sync_standby" {
			return member
		}
	}

	return PatroniMember{}
}

func (cluster *PatroniCluster) GetLeaderCandidate() PatroniMember {
	replicas := []PatroniMember{}
	for _, member := range cluster.Members {
		if member.Role == "sync_standby" {
			return member
		}

		if member.Role == "replica" {
			replicas = append(replicas, member)
		}
	}

	if len(replicas) == 0 {
		return PatroniMember{}
	}

	return replicas[rand.Intn(len(replicas))]
}

func (cluster *PatroniCluster) IsHealthy(expectedCount int) bool {
	for _, member := range cluster.Members {
		if (member.State != "running" && member.State != "streaming") || member.Lag < PatroniMemberLag(0) {
			return false
		}
	}

	return len(cluster.Members) == expectedCount
}

func getTlsConfigs(patrConf *config.PatroniClientConfig) (*tls.Config, error) {
	tlsConf := &tls.Config{
		InsecureSkipVerify: false,
	}

	//CA cert
	if patrConf.Auth.CaCert != "" {
		caCertContent, err := ioutil.ReadFile(patrConf.Auth.CaCert)
		if err != nil {
			return nil, errors.New(fmt.Sprintf("Failed to read patroni CA certificate file: %s", err.Error()))
		}
		roots := x509.NewCertPool()
		ok := roots.AppendCertsFromPEM(caCertContent)
		if !ok {
			return nil, errors.New("Failed to parse patroni CA certificat")
		}
		(*tlsConf).RootCAs = roots
	}

	certData, err := tls.LoadX509KeyPair(patrConf.Auth.ClientCert, patrConf.Auth.ClientKey)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Failed to patroni client certificate key pair: %s", err.Error()))
	}
	(*tlsConf).Certificates = []tls.Certificate{certData}

	return tlsConf, nil
}

type PatroniClient struct {
	client   *http.Client
	endpoint string
	log      logger.Logger
	conf     *config.PatroniClientConfig
}

func NewPatroniClient(patrConf *config.PatroniClientConfig, log logger.Logger) (PatroniClient, error) {
	tlsConf, tlsConfErr := getTlsConfigs(patrConf)
	if tlsConfErr != nil {
		return PatroniClient{}, tlsConfErr
	}

	return PatroniClient{
		client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConf,
				TLSHandshakeTimeout: patrConf.ConnectionTimeout,
				IdleConnTimeout: patrConf.RequestTimeout,
				ResponseHeaderTimeout: patrConf.RequestTimeout,
			},
		},
		endpoint: patrConf.Endpoint,
		log: log,
		conf: patrConf,
	}, nil
}

func (pClient *PatroniClient) GetCluster() (PatroniCluster, error) {
	var cluster PatroniCluster

	res, resErr := pClient.client.Get(fmt.Sprintf("https://%s/cluster", pClient.endpoint))
	if resErr != nil {
		return cluster, resErr
	}

	body, bodyErr := ioutil.ReadAll(res.Body)
	if bodyErr != nil {
		return cluster, bodyErr
	}

	parseErr := json.Unmarshal(body, &cluster)
	
	return cluster, parseErr
}

type switchoverReqBody struct {
	Leader    string `json:"leader"`
	Candidate string `json:"candidate,omitempty"`
}

type SwitchoverResult struct {
	PreviousLeader string
	NewLeader     string
}

func (pClient *PatroniClient) Switchover(excludeLeader bool) (SwitchoverResult, error) {
	result := SwitchoverResult{}
	
	cluster, clusterErr := pClient.GetCluster()
	if clusterErr != nil {
		return result, clusterErr
	}

	result.PreviousLeader = cluster.GetLeader().Name

	reqBody := switchoverReqBody{
		Leader: cluster.GetLeader().Name,
		Candidate: "",
	}

	if excludeLeader {
		candidate := cluster.GetLeaderCandidate()
		if candidate.Name == "" {
			return result, errors.New("Could not do a swichover that excludes leader: Not suitable candidate was found")
		}
		reqBody.Candidate = candidate.Name
	}

	body, bodyErr := json.Marshal(reqBody)
	if bodyErr != nil {
		return result, bodyErr
	}

	res, resErr := pClient.client.Post(fmt.Sprintf("https://%s/switchover", pClient.endpoint), "application/json", bytes.NewBuffer(body))
	if resErr != nil {
		return result, resErr
	}

	body, bodyErr = ioutil.ReadAll(res.Body)
	if bodyErr != nil {
		return result, bodyErr
	}

	if switchoverResponseRegex.MatchString(string(body)) {
		match := switchoverResponseRegex.FindStringSubmatch(string(body))
		result.NewLeader = string(match[1])
	}

	return result, nil
}

func (pClient *PatroniClient) WaitForHealthy(timeout time.Duration, expectedCount int) error {
	deadline := time.NewTimer(timeout)

	cluster, clusterErr := pClient.GetCluster()

	for clusterErr != nil || !cluster.IsHealthy(expectedCount) {
		select {
		case <-deadline.C:
			return errors.New(fmt.Sprintf("Cluster was not healthy within the deadline of %s", timeout.String()))
		default:
		}

		cluster, clusterErr = pClient.GetCluster()
		if clusterErr != nil {
			cli, cliErr := NewPatroniClient(pClient.conf, pClient.log)
			if cliErr == nil {
				(*pClient) = cli
			}
		}
	}

	return nil
}

func (pClient *PatroniClient) ForceLeaderChange(timeout time.Duration) error {
	begining := time.Now()

	cluster, clusterErr := pClient.GetCluster()
	if clusterErr != nil {
		return clusterErr
	}

	switchRes, switchErr := pClient.Switchover(true)
	if switchErr != nil {
		return switchErr
	}

	healthErr := pClient.WaitForHealthy(timeout, len(cluster.Members))
	if healthErr != nil {
		return healthErr
	}

	if switchRes.NewLeader == "" {
		cluster, clusterErr = pClient.GetCluster()
		if clusterErr != nil {
			return clusterErr
		}
		switchRes.NewLeader = cluster.GetLeader().Name
	}


	pClient.log.Infof("Switchover from leader \"%s\" to leader \"%s\" with healthy cluster in %s", switchRes.PreviousLeader, switchRes.NewLeader, time.Now().Sub(begining).String())

	return nil
}