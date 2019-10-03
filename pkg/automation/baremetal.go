package automation

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	getter "github.com/hashicorp/go-getter"
	yaml "gopkg.in/yaml.v2"
)

var (
	automationRemoteSource string
)

func init() {
	automationRemoteSource = "github.com/redhat-nfvpe/kni-upi-lab.git"
}

type baremetalAutomatedDeployment struct {
	siteBuildPath string
	siteName      string
	siteRepo      string
}

func newBaremetal(siteBuildPath string, siteName string, siteRepo string) (AutomatedDeploymentInterface, error) {
	// Examine site's site-config and determine if automation is even possible for this site
	siteConfigSourcePath := fmt.Sprintf("%s/%s/site/00_install-config/site-config.yaml", siteBuildPath, siteName)
	filename, _ := filepath.Abs(siteConfigSourcePath)
	siteConfigFile, err := ioutil.ReadFile(filename)

	if err != nil {
		return nil, fmt.Errorf("Automation: newBaremetal: error reading site-config.yaml: %s", err)
	}

	var siteConfig map[string]interface{}

	err = yaml.Unmarshal(siteConfigFile, &siteConfig)

	if err != nil {
		return nil, fmt.Errorf("Automation: newBaremetal: error unmarshalling site-config.yaml: %s", err)
	}

	// Check unmarshalled YAML for a "provisioningInfrastructure" map.  If it has one, this indicates
	// that this site supports automation.  If it doesn't have one, this isn't necessarily an error,
	// (depending on what is calling into the automation package in this context) so just return nils.
	if _, ok := siteConfig["provisioningInfrastructure"].(map[interface{}]interface{}); !ok {
		return nil, nil
	}

	return baremetalAutomatedDeployment{
		siteBuildPath: siteBuildPath,
		siteName:      siteName,
		siteRepo:      siteRepo,
	}, nil
}

func (bad baremetalAutomatedDeployment) PrepareBastion() error {
	// Download repo
	automationDestination := fmt.Sprintf("%s/%s/baremetal_automation", bad.siteBuildPath, bad.siteName)

	// Clear baremetal automation repo if it already exists
	os.RemoveAll(automationDestination)

	log.Printf("baremetalAutomatedDeployment: PrepareBastion: downloading baremetal automation repo (%s)\n", automationRemoteSource)

	client := &getter.Client{Src: automationRemoteSource, Dst: automationDestination, Mode: getter.ClientModeAny}
	err := client.Get()

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: PrepareBastion: error cloning baremetal automation repository: %s", err)
	}

	// Copy the site's site-config.yaml into the automation repo
	siteConfigSource, err := os.Open(fmt.Sprintf("%s/%s/site/00_install-config/site-config.yaml", bad.siteBuildPath, bad.siteName))

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: PrepareBastion: error opening source site config file: %s", err)
	}

	defer siteConfigSource.Close()

	// Remove the existing automation site config, if any
	siteConfigDestinationPath := fmt.Sprintf("%s/cluster/site-config.yaml", automationDestination)
	os.RemoveAll(siteConfigDestinationPath)

	siteConfigDestination, err := os.OpenFile(siteConfigDestinationPath, os.O_RDWR|os.O_CREATE, 0666)

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: PrepareBastion: error opening destination site config file: %s", err)
	}

	defer siteConfigDestination.Close()

	_, err = io.Copy(siteConfigDestination, siteConfigSource)

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: PrepareBastion: error writing destination site config file: %s", err)
	}

	// Execute automation's prep_bm_host script
	cmd := exec.Command(fmt.Sprintf("%s/prep_bm_host.sh", automationDestination))
	cmd.Dir = automationDestination
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	log.Println("baremetalAutomatedDeployment: PrepareBastion: running baremetal automation host preparation script...")

	err = cmd.Run()

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: PrepareBastion: error running baremetal automation host preparation script")
	}

	log.Println("baremetalAutomatedDeployment: PrepareBastion: finished running automation host preparation script")

	return nil
}

func (bad baremetalAutomatedDeployment) DeployMasters() error {
	// TODO
	return nil
}

func (bad baremetalAutomatedDeployment) DeployWorkers() error {
	// TODO
	return nil
}
