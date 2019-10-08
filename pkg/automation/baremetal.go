package automation

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/otiai10/copy"

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

type scriptRunInstance struct {
	description string
	scriptFile  string
	args        []string
}

func newBaremetal(params AutomatedDeploymentParams) (AutomatedDeploymentInterface, error) {
	// Examine site's site-config and determine if automation is even possible for this site
	siteConfigSourcePath := fmt.Sprintf("%s/%s/site/00_install-config/site-config.yaml", params.SiteBuildPath, params.SiteName)
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
		siteBuildPath: params.SiteBuildPath,
		siteName:      params.SiteName,
		siteRepo:      params.SiteRepo,
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
	sitePath := fmt.Sprintf("%s/%s", bad.siteBuildPath, bad.siteName)

	// Make sure final_manifests directory is available
	finalManifestsPath := fmt.Sprintf("%s/final_manifests", sitePath)

	_, err := os.Stat(finalManifestsPath)

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: DeployMasters: unable to access final manifests at %s: %s", finalManifestsPath, err)
	}

	// Make sure automation-required manifests are available (these YAMLs should have been copied
	// to the directory during prepare_manifests)
	automationManifestsPath := fmt.Sprintf("%s/automation", sitePath)

	_, err = os.Stat(automationManifestsPath)

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: DeployMasters: unable to access automation manifests at %s: %s", automationManifestsPath, err)
	}

	// Make sure the baremetal automation repo is locally available
	automationRepoPath := fmt.Sprintf("%s/baremetal_automation", sitePath)

	_, err = os.Stat(automationRepoPath)

	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("baremetalAutomatedDeployment: DeployMasters: unable to access local automation repo at %s: %s", automationRepoPath, err)
		}

		// Doesn't exist, so clone it?
		// NOTE: It should already exist, having been created during the "fetch_requirements" step
		log.Printf("baremetalAutomatedDeployment: DeployMasters: downloading missing baremetal automation repo (%s)\n", automationRemoteSource)

		client := &getter.Client{Src: automationRemoteSource, Dst: automationRepoPath, Mode: getter.ClientModeAny}
		err := client.Get()

		if err != nil {
			return fmt.Errorf("baremetalAutomatedDeployment: DeployMasters: error cloning baremetal automation repository: %s", err)
		}
	}

	// Copy final_manifests into the automation repo's ocp directory (the ocp
	// directory is the default location that the automation scripts use for
	// various openshift-install calls)
	err = copy.Copy(finalManifestsPath, fmt.Sprintf("%s/ocp", automationRepoPath))

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: DeployMasters: error copying final_manifests into automation ocp directory: %s", err)
	}

	// Now run the actual automation scripts
	err = bad.runConfigGenerationScripts(automationRepoPath, automationManifestsPath)

	if err != nil {
		return err
	}

	// Then start the containers
	err = bad.runContainers(automationRepoPath)

	if err != nil {
		return err
	}

	// Finally run terraform commands to begin cluster deployment
	err = bad.runTerraform(automationRepoPath, "cluster")

	if err != nil {
		return err
	}

	log.Printf("baremetalAutomatedDeployment: DeployMasters: bootstrap and master(s) deploy initiated...\n")

	return nil
}

// automationRepoPath: contains path to automation repo directory
func (bad baremetalAutomatedDeployment) runContainers(automationRepoPath string) error {
	// Add scripts to run
	scripts := []scriptRunInstance{}

	scripts = append(scripts, scriptRunInstance{
		description: "dnsmasq provisioning container start",
		scriptFile:  "gen_config_prov.sh",
		args:        []string{"start"},
	})

	scripts = append(scripts, scriptRunInstance{
		description: "dnsmasq baremetal container start",
		scriptFile:  "gen_config_bm.sh",
		args:        []string{"start"},
	})

	// Need to make sure haproxy is built before running
	scripts = append(scripts, scriptRunInstance{
		description: "haproxy container build",
		scriptFile:  "gen_haproxy.sh",
		args:        []string{"build"},
	})

	scripts = append(scripts, scriptRunInstance{
		description: "haproxy container start",
		scriptFile:  "gen_haproxy.sh",
		args:        []string{"start"},
	})

	scripts = append(scripts, scriptRunInstance{
		description: "coredns container start",
		scriptFile:  "gen_coredns.sh",
		args:        []string{"start"},
	})

	scripts = append(scripts, scriptRunInstance{
		description: "matchbox container start",
		scriptFile:  "gen_matchbox.sh",
		args:        []string{"start"},
	})

	log.Printf("baremetalAutomatedDeployment: runContainers: starting bastion containers...\n")

	err := bad.runScripts(automationRepoPath, scripts)

	if err != nil {
		return err
	}

	log.Printf("baremetalAutomatedDeployment: runContainers: bastion containers successfully started\n")

	return nil
}

// automationRepoPath: contains path to automation repo directory
// automationManifestsPath: contains path to directory containing site-config.yaml, install-config.yaml
//                          and any required credential secret yamls
func (bad baremetalAutomatedDeployment) runConfigGenerationScripts(automationRepoPath string, automationManifestsPath string) error {
	// Add scripts to run
	scripts := []scriptRunInstance{}

	commonArgs := []string{
		fmt.Sprintf("-m%s", automationManifestsPath),
	}

	scripts = append(scripts, scriptRunInstance{
		description: "dnsmasq provisioning config generation",
		scriptFile:  "gen_config_prov.sh",
		args:        commonArgs,
	})

	scripts = append(scripts, scriptRunInstance{
		description: "dnsmasq baremetal config generation",
		scriptFile:  "gen_config_bm.sh",
		args:        commonArgs,
	})

	scripts = append(scripts, scriptRunInstance{
		description: "coredns config generation",
		scriptFile:  "gen_coredns.sh",
		args:        append([]string{"all"}, commonArgs...),
	})

	scripts = append(scripts, scriptRunInstance{
		description: "haproxy config generation",
		scriptFile:  "gen_haproxy.sh",
		args:        append(commonArgs, "gen-config"),
	})

	scripts = append(scripts, scriptRunInstance{
		description: "matchbox repo generation",
		scriptFile:  "gen_matchbox.sh",
		args:        append([]string{"repo"}, commonArgs...),
	})

	scripts = append(scripts, scriptRunInstance{
		description: "matchbox data generation",
		scriptFile:  "gen_matchbox.sh",
		args:        append([]string{"data"}, commonArgs...),
	})

	scripts = append(scripts, scriptRunInstance{
		description: "terraform cluster/work config generation",
		scriptFile:  "gen_terraform.sh",
		args:        append([]string{"all"}, commonArgs...),
	})

	scripts = append(scripts, scriptRunInstance{
		description: "terraform installation",
		scriptFile:  "gen_terraform.sh",
		args:        append([]string{"install"}, commonArgs...),
	})

	scripts = append(scripts, scriptRunInstance{
		description: "ignition config generation",
		scriptFile:  "gen_ignition.sh",
		args:        append([]string{"create-output"}, commonArgs...),
	})

	log.Printf("baremetalAutomatedDeployment: runConfigGenerationScripts: generating configuration...\n")

	err := bad.runScripts(automationRepoPath, scripts)

	if err != nil {
		return err
	}

	log.Printf("baremetalAutomatedDeployment: runConfigGenerationScripts: configuration successfully generated\n")

	return nil
}

func (bad baremetalAutomatedDeployment) DeployWorkers() error {
	sitePath := fmt.Sprintf("%s/%s", bad.siteBuildPath, bad.siteName)
	automationRepoPath := fmt.Sprintf("%s/baremetal_automation", sitePath)

	_, err := os.Stat(automationRepoPath)

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: DeployWorkers: unable to access local automation repo at %s: %s", automationRepoPath, err)
	}

	// Finally run terraform commands to begin workers deployment
	err = bad.runTerraform(automationRepoPath, "workers")

	if err != nil {
		return err
	}

	log.Printf("baremetalAutomatedDeployment: DeployWorkers: worker(s) deploy initiated...\n")

	return nil
}

func (bad baremetalAutomatedDeployment) runTerraform(automationRepoPath string, targetType string) error {
	terraformPath := fmt.Sprintf("%s/terraform/%s", automationRepoPath, targetType)

	log.Printf("baremetalAutomatedDeployment: runTerraform: initializing terraform...\n")

	// Init
	cmd := exec.Command("terraform", "init")
	cmd.Dir = terraformPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: runTerraform: error running baremetal automation %s terraform init: %s", targetType, err)
	}

	log.Printf("baremetalAutomatedDeployment: runTerraform: terraform successfully initialized\n")
	log.Printf("baremetalAutomatedDeployment: runTerraform: applying terraform...\n")

	// Apply
	cmd = exec.Command("terraform", "apply", "--auto-approve")
	cmd.Dir = terraformPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: runTerraform: error running baremetal automation %s terraform apply: %s", targetType, err)
	}

	log.Printf("baremetalAutomatedDeployment: runTerraform: terraform successfully applied\n")

	return nil
}

func (bad baremetalAutomatedDeployment) runScripts(automationRepoPath string, scripts []scriptRunInstance) error {
	for _, script := range scripts {
		cmd := exec.Command(fmt.Sprintf("%s/scripts/%s", automationRepoPath, script.scriptFile), script.args...)
		cmd.Dir = automationRepoPath
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		log.Printf("baremetalAutomatedDeployment: runScripts: running %s script...\n", script.description)

		err := cmd.Run()

		if err != nil {
			return fmt.Errorf("baremetalAutomatedDeployment: runScripts: error running %s script: %s", script.description, err)
		}

		log.Printf("baremetalAutomatedDeployment: runScripts: finished running %s script\n", script.description)
	}

	return nil
}
