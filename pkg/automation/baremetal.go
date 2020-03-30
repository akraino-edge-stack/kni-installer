package automation

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gerrit.akraino.org/kni/installer/pkg/utils"
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

type terraformOperation string

const (
	terraformApply   terraformOperation = "apply"
	terraformDestroy terraformOperation = "destroy"
	terraformInit    terraformOperation = "init"
)

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

func (bad baremetalAutomatedDeployment) PrepareAutomation(requirements map[string]string) error {
	// Download repo
	automationDestination := fmt.Sprintf("%s/%s/baremetal_automation", bad.siteBuildPath, bad.siteName)

	// Clear baremetal automation repo if it already exists
	os.RemoveAll(automationDestination)

	log.Printf("baremetalAutomatedDeployment: PrepareAutomation: downloading baremetal automation repo (%s)...\n", automationRemoteSource)

	client := &getter.Client{Src: automationRemoteSource, Dst: automationDestination, Mode: getter.ClientModeAny}
	err := client.Get()

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: PrepareAutomation: error cloning baremetal automation repository: %s", err)
	}

	// Copy the site's site-config.yaml into the automation repo
	siteConfigSource, err := os.Open(fmt.Sprintf("%s/%s/site/00_install-config/site-config.yaml", bad.siteBuildPath, bad.siteName))

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: PrepareAutomation: error opening source site config file: %s", err)
	}

	defer siteConfigSource.Close()

	// Remove the existing automation site config, if any
	siteConfigDestinationPath := fmt.Sprintf("%s/cluster/site-config.yaml", automationDestination)
	os.RemoveAll(siteConfigDestinationPath)

	siteConfigDestination, err := os.OpenFile(siteConfigDestinationPath, os.O_RDWR|os.O_CREATE, 0666)

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: PrepareAutomation: error opening destination site config file: %s", err)
	}

	defer siteConfigDestination.Close()

	_, err = io.Copy(siteConfigDestination, siteConfigSource)

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: PrepareAutomation: error writing destination site config file: %s", err)
	}

	// Create "requirements" directory in the automation repo (needed for later commands)
	requirementsPath := fmt.Sprintf("%s/requirements", automationDestination)
	os.Mkdir(requirementsPath, 0755)

	log.Printf("baremetalAutomatedDeployment: PrepareAutomation: finished downloading baremetal automation repo (%s)\n", automationRemoteSource)

	log.Printf("baremetalAutomatedDeployment: PrepareAutomation: injecting version selections into automation repo...\n")

	// Examine requirements to check for oc or openshift-install version selection.
	// If they are found, inject them into the automation's images_and_binaries.sh script
	// to override the default
	binaryVersionsPath := fmt.Sprintf("%s/images_and_binaries.sh", automationDestination)

	for requirementName, requirementSource := range requirements {
		switch requirementName {
		case "oc":
			err = utils.ReplaceFileText(binaryVersionsPath, "OCP_CLIENT_BINARY_URL=\"\"", fmt.Sprintf("OCP_CLIENT_BINARY_URL=\"%s\"", requirementSource))

			if err != nil {
				return fmt.Errorf("baremetalAutomatedDeployment: PrepareAutomation: error injecting oc binary version: %s", err)
			}
		case "openshift-install":
			err = utils.ReplaceFileText(binaryVersionsPath, "OCP_INSTALL_BINARY_URL=\"\"", fmt.Sprintf("OCP_INSTALL_BINARY_URL=\"%s\"", requirementSource))

			if err != nil {
				return fmt.Errorf("baremetalAutomatedDeployment: PrepareAutomation: error injecting openshift-install binary version: %s", err)
			}
		}
	}

	// Check site-config.yaml's config block for a releaseImageOverride.  If found, extract
	// the image's tag to get the requested version for RHCOS images, and inject that into
	// automation's common.sh script to override the default
	var siteConfig map[string]interface{}
	siteConfigFile, err := ioutil.ReadFile(siteConfigDestinationPath)

	err = yaml.Unmarshal(siteConfigFile, &siteConfig)

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: PrepareAutomation: error unmarshalling site-config.yaml: %s", err)
	}
	rhcosVersionsPath := fmt.Sprintf("%s/common.sh", automationDestination)

	if config, ok := siteConfig["config"].(map[interface{}]interface{}); ok {
		if releaseImageOverride, ok := config["releaseImageOverride"].(string); ok {
			parts := strings.Split(releaseImageOverride, ":")

			if len(parts) == 2 {
				err = utils.ReplaceFileText(rhcosVersionsPath, "OPENSHIFT_RHCOS_MAJOR_REL=\"\"", fmt.Sprintf("OPENSHIFT_RHCOS_MAJOR_REL=\"%s\"", parts[1]))

				if err != nil {
					return fmt.Errorf("baremetalAutomatedDeployment: PrepareAutomation: error injecting RHCOS image version: %s", err)
				}
			}
		}

		if virtualizedInstall, ok := config["virtualizedInstall"].(string); ok {

			err = utils.ReplaceFileText(rhcosVersionsPath, "VIRTUALIZED_INSTALL=false", fmt.Sprintf("VIRTUALIZED_INSTALL=%s", virtualizedInstall))

			if err != nil {
				return fmt.Errorf("baremetalAutomatedDeployment: PrepareAutomation: error injecting virtualized install setting: %s", err)
			}
		}
	}

	log.Printf("baremetalAutomatedDeployment: PrepareAutomation: finished injecting version selections into automation repo\n")

	return nil
}

func (bad baremetalAutomatedDeployment) FinalizeAutomationPreparation() error {
	// Copy finalized manifests into the baremetal automation repo directory
	automationManifestSource := fmt.Sprintf("%s/%s/automation", bad.siteBuildPath, bad.siteName)
	automationDestination := fmt.Sprintf("%s/%s/baremetal_automation", bad.siteBuildPath, bad.siteName)
	automationManifestDestination := fmt.Sprintf("%s/cluster", automationDestination)

	// Need to remove all files copied to the site's cluster manifests staging dir that do not
	// have associated "kind" content within them, as the baremetal automation repo logic will
	// not tolerate anything without a "kind"
	err := filepath.Walk(automationManifestSource, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Not interested in directories, so just keep walking
		if info.IsDir() {
			return nil
		}

		filename, _ := filepath.Abs(path)
		fileBytes, err := ioutil.ReadFile(filename)

		if err != nil {
			// File can't be read, so skip it
			return nil
		}

		// Empty file is useless, so just keep walking
		if len(fileBytes) == 0 {
			return nil
		}

		var fileObject map[string]interface{}

		err = yaml.Unmarshal(fileBytes, &fileObject)

		if err != nil {
			// File does not have the proper YAML format we are looking for, so skip it
			return nil
		}

		// Check unmarshalled file for a "kind" key
		if _, ok := fileObject["kind"]; ok {
			// Kind found, so we need to copy this into the baremetal automation
			// cluster manifests directory
			err = ioutil.WriteFile(fmt.Sprintf("%s/%s", automationManifestDestination, info.Name()), fileBytes, 0644)

			if err != nil {
				return err
			}

			log.Printf("baremetalAutomatedDeployment: FinalizeAutomationPreparation: copied %s to baremetal repo\n", info.Name())
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: FinalizeAutomationPreparation: error copying finalized manifests to automation repo: %s", err)
	}

	// Copy required versions of oc and openshift-install into the automation repo's "requirements"
	// directory so that they're used with later automation calls
	log.Printf("baremetalAutomatedDeployment: FinalizeAutomationPreparation: injecting OpenShift binaries for automation repo...\n")

	requirementsSource := fmt.Sprintf("%s/%s/requirements", bad.siteBuildPath, bad.siteName)
	requirementsDestination := fmt.Sprintf("%s/%s/baremetal_automation/requirements/.", bad.siteBuildPath, bad.siteName)
	prepHostSkipOcpBinariesArg := "--skip-ocp-binaries"

	for _, requirement := range []string{"oc", "openshift-install"} {
		requirementFullPath := fmt.Sprintf("%s/%s", requirementsSource, requirement)

		_, err := os.Stat(requirementFullPath)

		if err != nil {
			// Requirement was missing, so warn the user (in this case, automation will use
			// the OCP binaries pulled by the call to prep_bm_host.sh below)
			log.Printf("WARNING: '%s' requirement not specified; automation will use the default selected version for OpenShift binaries!", requirement)
			prepHostSkipOcpBinariesArg = ""
			break
		}

		utils.ExecuteCommand("", nil, true, false, "cp", requirementFullPath, requirementsDestination)
	}

	log.Printf("baremetalAutomatedDeployment: FinalizeAutomationPreparation: finished injecting OpenShift binaries for automation repo\n")

	// Execute automation's prep_bm_host script now that all manifests have been
	// copied to the baremetal automation repo's cluster manifests directory
	cmd := exec.Command(fmt.Sprintf("%s/prep_bm_host.sh", automationDestination), prepHostSkipOcpBinariesArg)
	cmd.Dir = automationDestination
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	log.Println("baremetalAutomatedDeployment: FinalizeAutomationPreparation: running baremetal automation host preparation script...")

	err = cmd.Run()

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: FinalizeAutomationPreparation: error running baremetal automation host preparation script: %s", err)
	}

	log.Println("baremetalAutomatedDeployment: FinalizeAutomationPreparation: finished running automation host preparation script")

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
		return fmt.Errorf("baremetalAutomatedDeployment: DeployMasters: unable to access local automation repo at %s: %s", automationRepoPath, err)
	}

	// Copy final_manifests into the automation repo's ocp directory (the ocp
	// directory is the default location that the automation scripts use for
	// various openshift-install calls)
	err = copy.Copy(finalManifestsPath, fmt.Sprintf("%s/ocp", automationRepoPath))

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: DeployMasters: error copying final_manifests into automation ocp directory: %s", err)
	}

	// Now run the actual automation scripts, including ignition-generation
	err = bad.runConfigGenerationScripts(automationRepoPath, true)

	if err != nil {
		return err
	}

	// Then start the containers
	err = bad.runContainers(automationRepoPath)

	if err != nil {
		return err
	}

	// Finally run terraform commands to begin cluster deployment
	err = bad.runTerraform(automationRepoPath, "cluster", terraformApply)

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
func (bad baremetalAutomatedDeployment) runConfigGenerationScripts(automationRepoPath string, includeIgnition bool) error {
	// Add scripts to run
	scripts := []scriptRunInstance{}

	// Placeholder
	commonArgs := []string{}

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

	if includeIgnition {
		scripts = append(scripts, scriptRunInstance{
			description: "ignition config generation",
			scriptFile:  "gen_ignition.sh",
			args:        append([]string{"create-output"}, commonArgs...),
		})
	}

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

	// Make sure automation-required manifests are available (these YAMLs should have been copied
	// to the directory during prepare_manifests)
	automationManifestsPath := fmt.Sprintf("%s/automation", sitePath)

	_, err = os.Stat(automationManifestsPath)

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: DeployWorkers: unable to access automation manifests at %s: %s", automationManifestsPath, err)
	}

	// Now run the actual automation scripts, minus ignition-generation
	err = bad.runConfigGenerationScripts(automationRepoPath, false)

	if err != nil {
		return err
	}

	// Then start the containers
	err = bad.runContainers(automationRepoPath)

	if err != nil {
		return err
	}

	// Finally run terraform commands to begin workers deployment
	err = bad.runTerraform(automationRepoPath, "workers", terraformApply)

	if err != nil {
		return err
	}

	log.Printf("baremetalAutomatedDeployment: DeployWorkers: worker(s) deploy initiated...\n")

	return nil
}

func (bad baremetalAutomatedDeployment) DestroyCluster() error {
	sitePath := fmt.Sprintf("%s/%s", bad.siteBuildPath, bad.siteName)
	automationRepoPath := fmt.Sprintf("%s/baremetal_automation", sitePath)

	_, err := os.Stat(automationRepoPath)

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: DestroyCluster: unable to access local automation repo at %s: %s", automationRepoPath, err)
	}

	// Destroy workers via terraform
	err = bad.runTerraform(automationRepoPath, "workers", terraformDestroy)

	if err != nil {
		return err
	}

	// Destroy masters via terraform
	err = bad.runTerraform(automationRepoPath, "cluster", terraformDestroy)

	if err != nil {
		return err
	}

	// Remove bastion (provisioning host) containers
	scripts := []scriptRunInstance{}

	commonArgs := []string{"remove"}

	scripts = append(scripts, scriptRunInstance{
		description: "dnsmasq provisioning container removal",
		scriptFile:  "gen_config_prov.sh",
		args:        commonArgs,
	})

	scripts = append(scripts, scriptRunInstance{
		description: "dnsmasq baremetal container removal",
		scriptFile:  "gen_config_bm.sh",
		args:        commonArgs,
	})

	scripts = append(scripts, scriptRunInstance{
		description: "haproxy container removal",
		scriptFile:  "gen_haproxy.sh",
		args:        commonArgs,
	})

	scripts = append(scripts, scriptRunInstance{
		description: "coredns container removal",
		scriptFile:  "gen_coredns.sh",
		args:        commonArgs,
	})

	scripts = append(scripts, scriptRunInstance{
		description: "matchbox container removal",
		scriptFile:  "gen_matchbox.sh",
		args:        commonArgs,
	})

	err = bad.runScripts(automationRepoPath, scripts)

	if err != nil {
		return err
	}

	// Clear config directories
	dirs := []string{
		"build",
		"coredns",
		"dnsmasq",
		"haproxy",
		"ocp",
	}

	for _, dir := range dirs {
		os.RemoveAll(fmt.Sprintf("%s/%s", automationRepoPath, dir))
	}

	log.Printf("baremetalAutomatedDeployment: DestroyCluster: cluster teardown completed\n")

	return nil
}

func (bad baremetalAutomatedDeployment) runTerraform(automationRepoPath string, targetType string, operation terraformOperation) error {
	terraformPath := fmt.Sprintf("%s/terraform/%s", automationRepoPath, targetType)

	log.Printf("baremetalAutomatedDeployment: runTerraform: initializing terraform...\n")

	// Init
	cmd := exec.Command("terraform", string(terraformInit))
	cmd.Dir = terraformPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: runTerraform: error running baremetal automation %s terraform init: %s", targetType, err)
	}

	log.Printf("baremetalAutomatedDeployment: runTerraform: terraform successfully initialized\n")
	log.Printf("baremetalAutomatedDeployment: runTerraform: running terraform %s...\n", operation)

	// Apply
	cmd = exec.Command("terraform", string(operation), "--auto-approve")
	cmd.Dir = terraformPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()

	if err != nil {
		return fmt.Errorf("baremetalAutomatedDeployment: runTerraform: error running baremetal automation %s terraform %s: %s", targetType, operation, err)
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
