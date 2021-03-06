package site

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gerrit.akraino.org/kni/installer/pkg/automation"
	"gerrit.akraino.org/kni/installer/pkg/manifests"
	"gerrit.akraino.org/kni/installer/pkg/requirements"
	"gerrit.akraino.org/kni/installer/pkg/utils"
	getter "github.com/hashicorp/go-getter"
	"github.com/otiai10/copy"
	"gopkg.in/yaml.v2"
)

// Site : Structure that contains the settings needed for managing a site
type Site struct {
	siteRepo  string
	siteName  string
	buildPath string
}

// New constructor for the generator
func New(siteRepo string, buildPath string) Site {
	// given a site repo, extract the site name from the path
	baseName := path.Base(siteRepo)
	suffixList := [2]string{".git", ".tar.gz"}

	siteName := baseName
	for _, suffix := range suffixList {
		siteName = strings.TrimSuffix(siteName, suffix)
	}

	s := Site{siteRepo, siteName, buildPath}
	return s
}

// new constructor but just passing the name and path
func NewWithName(siteName string, buildPath string) Site {
	s := Site{"", siteName, buildPath}
	return s
}

// given a site repo, downloads the content and places into buildPath
func (s Site) DownloadSite() {
	if s.siteRepo != "" {
		// Clone the site repository
		log.Printf("Cloning the site repository from %s\n", s.siteRepo)
		siteLayerPath := fmt.Sprintf("%s/%s/site", s.buildPath, s.siteName)
		os.RemoveAll(siteLayerPath)

		if strings.HasPrefix(s.siteRepo, "file://") {
			// just do a local copy
			var envVars []string
			os.MkdirAll(siteLayerPath, 0775)
			originPath := fmt.Sprintf("%s/.", s.siteRepo[7:len(s.siteRepo)])
			utils.ExecuteCommand("", envVars, true, false, "cp", "-a", originPath, siteLayerPath)
		} else {
			client := &getter.Client{Src: s.siteRepo, Dst: siteLayerPath, Mode: getter.ClientModeAny}
			err := client.Get()
			if err != nil {
				log.Fatalf("Error cloning site repository: %s\n", err)
			}
		}
	} else {
		log.Fatalf("Site repository does not exist for the site %s\n", s.siteName)
	}

}

// retrieves the given profile used in a site
func (s Site) GetProfileFromSite() (string, string, string) {
	sitePath := fmt.Sprintf("%s/%s", s.buildPath, s.siteName)

	profileFile := fmt.Sprintf("%s/site/00_install-config/kustomization.yaml", sitePath)

	if _, err := os.Stat(profileFile); err == nil {
		// parse yaml and extract base
		yamlContent, err := ioutil.ReadFile(profileFile)
		if err != nil {
			log.Fatalf("Error reading profile file: %s\n", err)
		}

		profileSettings := &map[string][]interface{}{}
		err = yaml.Unmarshal(yamlContent, &profileSettings)
		if err != nil {
			log.Fatalf("Error parsing profile yaml file: %s\n", err)
		}
		bases := (*profileSettings)["bases"]
		profileRepo := fmt.Sprintf("%s", bases[0])

		// given the profile repo, we need to get the full path without file, and clone it

		// first extract the ref
		pos := strings.LastIndex(profileRepo, "?ref=")
		profileRef := ""
		if pos != -1 {
			adjustedPos := pos + len("?ref=")
			profileRef = profileRepo[adjustedPos:len(profileRepo)]
		}

		// then the name and path
		profileBits := strings.Split(profileRepo, "/")
		profileName := profileBits[len(profileBits)-2]
		profileLayerPath := strings.TrimSuffix(profileRepo, profileBits[len(profileBits)-1])
		if profileRef != "" {
			profileLayerPath = fmt.Sprintf("%s?ref=%s", profileLayerPath, profileRef)
		}

		return profileName, profileLayerPath, profileRef
	} else if os.IsNotExist(err) {
		log.Fatalf("File %s does not exist, exiting\n", profileFile)
	}

	return "", "", ""
}

// using the downloaded site content, fetches (and builds) the specified requirements,
// and also prepares the host for running scripts for the site's profile type
func (s Site) FetchRequirements(individualRequirements []string) {
	log.Printf("Downloading requirements for %s\n", s.siteName)
	sitePath := fmt.Sprintf("%s/%s", s.buildPath, s.siteName)

	// searches for file containing the profile of the blueprint
	profileName, profileLayerPath, _ := s.GetProfileFromSite()

	profileBuildPath := fmt.Sprintf("%s/%s", sitePath, profileName)
	log.Printf("Downloading profile repo from %s into %s\n", profileLayerPath, profileBuildPath)
	if strings.HasPrefix(profileLayerPath, "file://") {
		// just do a local copy
		var envVars []string
		os.MkdirAll(profileBuildPath, 0775)
		originPath := fmt.Sprintf("%s/.", profileLayerPath[7:len(profileLayerPath)])
		utils.ExecuteCommand("", envVars, true, false, "cp", "-a", originPath, profileBuildPath)
	} else {
		client := &getter.Client{Src: profileLayerPath, Dst: profileBuildPath, Mode: getter.ClientModeAny}
		err := client.Get()
		if err != nil {
			log.Fatalf("Error cloning profile repository: %s\n", err)
		}
	}

	// read yaml from requirements and start fetching the bits
	requirementsFile := fmt.Sprintf("%s/requirements.yaml", profileBuildPath)
	file, err := os.Open(requirementsFile)
	if err != nil {
		log.Fatalln("Error reading requirements file")
	}
	defer file.Close()

	parsedRequirements := map[string]string{}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		requirementsLine := scanner.Text()

		// requirements is composed of binary and source
		requirementsBits := strings.SplitN(strings.TrimSpace(requirementsLine), ":", 2)
		binaryName := strings.TrimSpace(requirementsBits[0])
		binarySource := strings.TrimSpace(requirementsBits[1])

		// Store requirement for use in call to prepareHostForAutomation, regardless of
		// what happens with the "individualRequirements" check below.  If automation is
		// in fact used later on, we want to honor the potential "oc" and "openshift-install"
		// binary versions set in the blueprint profile's "requirements.yaml"
		parsedRequirements[binaryName] = binarySource

		// if we have individual requirements list, check if we have the requirement on it. Otherwise, skip
		if len(individualRequirements) > 0 {
			foundReq := false
			for _, individualRequirement := range individualRequirements {
				if individualRequirement == binaryName {
					foundReq = true
					break
				}
			}
			if !foundReq {
				// skip this iteration
				log.Printf("Binary %s not found in list, skipping\n", binaryName)
				continue
			}
		}
		r := requirements.New(binaryName, binarySource, fmt.Sprintf("%s/requirements", sitePath))
		r.FetchRequirement()
	}

	// Prepares host automation for post-'prepare_manifests' execution (if any)
	err = s.prepareHostForAutomation(profileName, parsedRequirements)

	if err != nil {
		log.Fatal(err)
	}

	// remove profile folder
	os.RemoveAll(profileBuildPath)

}

// writes an env file, that needs to be sourced before running cluster install
func (s Site) WriteEnvFile() {
	envContents := ""

	// fist we check if release image override exists on site definition
	siteBuildPath := fmt.Sprintf("%s/%s", s.buildPath, s.siteName)
	installYaml := fmt.Sprintf("%s/site/00_install-config/site-config.yaml", siteBuildPath)

	var configFileObj map[interface{}]interface{}
	b, err := ioutil.ReadFile(installYaml)
	if err != nil {
		log.Fatalf("Error reading site config file: %s\n", err)
	}

	// parse the yaml and check for key value
	err = yaml.Unmarshal(b, &configFileObj)
	if err != nil {
		log.Printf("Error parsing manifest: %s\n", err)
	}

	envVars, ok := configFileObj["config"].(map[interface{}]interface{})
	if ok {
		for index, element := range envVars {
			envContents = fmt.Sprintf("%sexport %s=%s\n", envContents, string(index.(string)), string(element.(string)))
		}
	}

	// write a profile.env in the siteBuildPath
	err = ioutil.WriteFile(fmt.Sprintf("%s/profile.env", siteBuildPath), []byte(envContents), 0644)
	if err != nil {
		log.Fatalf("Error writing profile.env file: %s\n", err)
	}
}

// given a site, download the repo dependencies
func (s Site) DownloadRepo(sitePath string, profileLayerPath string, profileRef string) {
	var blueprintRepo string
	var absoluteBlueprintRepo string
	var downloadRepo string

	// check if we have a file or git
	if strings.HasPrefix(profileLayerPath, "file://") {
		blueprintRepo = profileLayerPath

		// we need to extract the absolute repo, so we strip from profiles
		pos := strings.LastIndex(blueprintRepo, "/profiles")
		if pos != -1 {
			downloadRepo = blueprintRepo[0 : pos+1]
		}
		absoluteBlueprintRepo = downloadRepo
	} else {
		indexGit := strings.LastIndex(profileLayerPath, "//")
		if indexGit == -1 {
			blueprintRepo = profileLayerPath
			absoluteBlueprintRepo = profileLayerPath
		} else {
			blueprintRepo = profileLayerPath[0:indexGit]
			absoluteBlueprintRepo = profileLayerPath[0:(indexGit + 2)]
		}

		downloadRepo = blueprintRepo
		if profileRef != "" {
			downloadRepo = fmt.Sprintf("%s?ref=%s", blueprintRepo, profileRef)
		}
	}

	log.Printf("Downloading blueprint repo from %s\n", downloadRepo)
	blueprintDir := fmt.Sprintf("%s/blueprint", sitePath)
	os.RemoveAll(blueprintDir)

	if strings.HasPrefix(downloadRepo, "file://") {
		// just do a local copy
		var envVars []string
		os.MkdirAll(blueprintDir, 0775)
		originPath := fmt.Sprintf("%s/.", downloadRepo[7:len(downloadRepo)])
		utils.ExecuteCommand("", envVars, true, false, "cp", "-a", originPath, blueprintDir)
	} else {
		client := &getter.Client{Src: downloadRepo, Dst: blueprintDir, Mode: getter.ClientModeAny}
		err := client.Get()
		if err != nil {
			log.Fatalf("Error cloning profile repository: %s\n", err)
		}
	}

	// and now copy site inside the sites folder, replacing the absolute references to relative
	var envVars []string
	utils.ExecuteCommand("", envVars, true, false, "cp", "-R", fmt.Sprintf("%s/site", sitePath), fmt.Sprintf("%s/blueprint/sites/site", sitePath))

	filepath.Walk(fmt.Sprintf("%s/blueprint/sites/site", sitePath), func(path string, info os.FileInfo, err error) error {
		if err == nil {
			if info.Name() == "kustomization.yaml" {
				readKustomization, err := ioutil.ReadFile(path)
				if err != nil {
					log.Fatalf("Error opening kustomization file: %s\n", err)
				}

				newKustomization := strings.Replace(string(readKustomization), absoluteBlueprintRepo, "../../../", -1)
				if profileRef != "" {
					newKustomization = strings.Replace(newKustomization, fmt.Sprintf("?ref=%s", profileRef), "", -1)
				}

				err = ioutil.WriteFile(path, []byte(newKustomization), 0)
				if err != nil {
					log.Fatalf("Error writing modified kustomization file: %s\n", err)
				}
				return nil
			}

		} else {
			log.Fatalf("Error walking on site directory: %s\n", err)
		}

		return nil
	})
}

// using the downloaded site content, prepares the manifests for it, and also runs
// host preparation finalization scripts for site automation (if any)
func (s Site) PrepareManifests() {
	sitePath := fmt.Sprintf("%s/%s", s.buildPath, s.siteName)
	log.Printf("Preparing manifests for %s\n", s.siteName)

	// do the initial validation of pre-requisites
	utils.ValidateRequirements(s.buildPath, s.siteName)
	binariesPath := fmt.Sprintf("%s/requirements", sitePath)

	// retrieve profile name/path and clone the repo
	profileName, profileLayerPath, profileRef := s.GetProfileFromSite()
	s.DownloadRepo(sitePath, profileLayerPath, profileRef)

	// create automation sub-directory to store a copy of anything that might be
	// needed in the case of potential automation
	automationPath := fmt.Sprintf("%s/automation", sitePath)
	os.Mkdir(automationPath, 0755)

	// copy 00_install-config directory contents into automation sub-directory
	installConfigDirPath := fmt.Sprintf("%s/blueprint/sites/site/00_install-config", sitePath)
	err := copy.Copy(installConfigDirPath, automationPath)

	if err != nil {
		log.Fatalf("Error copying 00_install-config directory: %s\n", err)
	}

	// generate openshift-install manifests based on phase 00_install-config
	assetsPath := fmt.Sprintf("%s/generated_assets", sitePath)
	os.RemoveAll(assetsPath)
	os.Mkdir(assetsPath, 0755)

	out := utils.ApplyKustomize(fmt.Sprintf("%s/kustomize", binariesPath), installConfigDirPath)
	// check if we have any content and write to the target file
	if len(out) > 0 {
		err := ioutil.WriteFile(fmt.Sprintf("%s/install-config.yaml", assetsPath), out, 0644)
		if err != nil {
			log.Fatalf("Error writing final install-config file: %s\n", err)
		}

		// create a copy of final install-config.yaml in any site automation sub-directories
		// in case automation is later needed
		err = ioutil.WriteFile(fmt.Sprintf("%s/install-config.yaml", automationPath), out, 0644)
		if err != nil {
			log.Fatalf("Error writing final install-config file to automation assets directory: %s\n", err)
		}
	} else {
		log.Fatalln("Error, kustomize did not return any content")
	}

	// now generate the manifests
	var envVars []string
	utils.ExecuteCommand("", envVars, true, true, fmt.Sprintf("%s/openshift-install", binariesPath), "create", "manifests", fmt.Sprintf("--dir=%s", assetsPath), "--log-level", "debug")
	// iterate over all the generated files and create a kustomization file
	f, err := os.Create(fmt.Sprintf("%s/kustomization.yaml", assetsPath))
	if err != nil {
		log.Fatalf("Error creating kustomization file: %s\n", err)
	}
	defer f.Close()

	_, err = f.WriteString("resources:\n")
	if err != nil {
		log.Fatalf("Error writing kustomization file: %s\n", err)
	}

	filePatterns := []string{fmt.Sprintf("%s/manifests/*.yaml", assetsPath), fmt.Sprintf("%s/manifests/*.yml", assetsPath), fmt.Sprintf("%s/openshift/*.yaml", assetsPath)}
	for _, filePattern := range filePatterns {
		files, err := filepath.Glob(filePattern)
		if err != nil {
			log.Fatalf("Error reading manifest files: %s\n", err)
		}

		// iterate over each file, remove the absolute path and write it
		for _, fileName := range files {
			strippedName := strings.TrimPrefix(fileName, fmt.Sprintf("%s/", assetsPath))
			_, err := f.WriteString(fmt.Sprintf("- %s\n", strippedName))
			if err != nil {
				log.Fatalf("Error writing kustomization file: %s\n", err)
			}
		}
	}

	// move the content of the generated site to blueprint base
	os.Rename(fmt.Sprintf("%s/generated_assets/", sitePath), fmt.Sprintf("%s/blueprint/base/00_cluster/", sitePath))

	// apply kustomize on cluster-mods
	out = utils.ApplyKustomize(fmt.Sprintf("%s/kustomize", binariesPath), fmt.Sprintf("%s/blueprint/sites/site/01_cluster-mods", sitePath))
	if len(out) > 0 {
		// now apply modifications on the manifests
		resultStr := manifests.MergeManifests(string(out), sitePath)

		// Now that we have finalized our manifests, call automation finalization (if any)
		err = s.finalizeHostForAutomation(profileName)

		if err != nil {
			log.Fatalln(err)
		}

		// Finally, print manifest merge output
		fmt.Println(resultStr)
	} else {
		log.Fatalln("Error, kustomize did not return any content")
	}

}

// using the site contents, applies the workloads on it
func (s Site) ApplyWorkloads(kubeconfigFile string, retryCount int, delay int) {
	siteBuildPath := fmt.Sprintf("%s/%s", s.buildPath, s.siteName)

	// if we have kubeconfig, validate that exists
	if len(kubeconfigFile) > 0 {
		if _, err := os.Stat(kubeconfigFile); err != nil {
			log.Fatalln("Error: kubeconfig file %s does not exist\n", kubeconfigFile)
		}
	}
	binariesPath := fmt.Sprintf("%s/requirements", siteBuildPath)

	// retrieve profile path and clone the repo
	_, profileLayerPath, profileRef := s.GetProfileFromSite()
	s.DownloadRepo(siteBuildPath, profileLayerPath, profileRef)

	log.Printf("Applying workloads from %s/blueprint/sites/site/02_cluster-addons\n", siteBuildPath)
	out := utils.ApplyKustomize(fmt.Sprintf("%s/kustomize", binariesPath), fmt.Sprintf("%s/blueprint/sites/site/02_cluster-addons", siteBuildPath))
	if string(out) != "" {
		// now we can apply it
		utils.ApplyOc(fmt.Sprintf("%s/oc", binariesPath), out, kubeconfigFile, retryCount, delay)
	} else {
		log.Printf("No manifests found for %s/blueprint/sites/site/02_cluster-addons\n", siteBuildPath)
	}
	log.Printf("Applying workloads from %s/blueprint/sites/site/03_services\n", siteBuildPath)
	out = utils.ApplyKustomize(fmt.Sprintf("%s/kustomize", binariesPath), fmt.Sprintf("%s/blueprint/sites/site/03_services", siteBuildPath))
	if string(out) != "" {
		// now we can apply it
		utils.ApplyOc(fmt.Sprintf("%s/oc", binariesPath), out, kubeconfigFile, retryCount, delay)
	} else {
		log.Printf("No manifests found for %s/blueprint/sites/site/03_services\n", siteBuildPath)
	}
}

func (s Site) AutomateMastersDeployment() {
	// Run the automated deployment
	err := s.automateDeployment("masters")

	if err != nil {
		log.Fatalf("Site: AutomateMastersDeployment: Error attempting to run automated deployment: %s\n", err)
	}
}

func (s Site) AutomateWorkersDeployment() {
	// Run the automated deployment
	err := s.automateDeployment("workers")

	if err != nil {
		log.Fatalf("Site: AutomateWorkersDeployment: Error attempting to run automated deployment: %s\n", err)
	}
}

func (s Site) AutomateClusterDestroy() {
	// Get an automated deployment object
	automatedDeployment, err := s.getAutomatedDeployment()

	if err != nil {
		log.Fatalf("Site: AutomateClusterDestroy: Error attempting to acquire automated deploy object: %s\n", err)
	}

	// Run the automated cluster teardown
	err = automatedDeployment.DestroyCluster()

	if err != nil {
		log.Fatalf("Site: AutomateClusterDestroy: Error attempting to run automated cluster destroy: %s\n", err)
	}
}

func (s Site) automateDeployment(deploymentType string) error {
	// Get an automated deployment object
	automatedDeployment, err := s.getAutomatedDeployment()

	if err != nil {
		return err
	}

	// Act based on the requested deployment type
	switch deploymentType {
	case "masters":
		return automatedDeployment.DeployMasters()
	case "workers":
		return automatedDeployment.DeployWorkers()
	default:
		return fmt.Errorf("Site: automateDeployment: unknown deployment type: %s", deploymentType)
	}
}

// Returns an AutomatedDeploymentInterface for use with automation operations
func (s Site) getAutomatedDeployment() (automation.AutomatedDeploymentInterface, error) {
	// Get profile name
	profileName, _, _ := s.GetProfileFromSite()

	// Get the profile type
	// NOTE: This call also checks whether the site repo exists locally, so there is no
	//       need to check that here
	profileType, err := s.getProfileType(profileName)

	if err != nil {
		return nil, fmt.Errorf("Site: getAutomatedDeployment: Error acquiring site profile type: %s", err)
	}

	// Create an automated deployment instance
	automatedDeploymentParams := automation.AutomatedDeploymentParams{
		ProfileType:   profileType,
		SiteBuildPath: s.buildPath,
		SiteName:      s.siteName,
		SiteRepo:      s.siteRepo,
	}

	automatedDeployment, err := automation.New(automatedDeploymentParams)

	if err != nil {
		return nil, fmt.Errorf("Site: getAutomatedDeployment: Error creating automated deployment instance: %s", err)
	}

	// If nil is returned for automatedDeployment, then this particular site does
	// not contain the necessary config required to automate its deployment
	if automatedDeployment == nil {
		return nil, fmt.Errorf("Site: getAutomatedDeployment: automated deployment not supported for site '%s'", s.siteName)
	}

	return automatedDeployment, nil
}

// Determines site profile type based on blueprint profile contents
func (s Site) getProfileType(profileName string) (string, error) {
	if profileName == "" || s.buildPath == "" || s.siteName == "" {
		return "", errors.New("Site: getProfileType: profile name, build path and/or site name missing")
	}

	sitePath := fmt.Sprintf("%s/%s", s.buildPath, s.siteName)
	installConfigDirPath := fmt.Sprintf("%s/%s/00_install-config/", sitePath, profileName)

	// Check that blueprint profile install config directory is available
	_, err := os.Stat(installConfigDirPath)

	if err != nil {
		// Check the other possible location
		installConfigDirPath = fmt.Sprintf("%s/blueprint/profiles/%s/00_install-config", sitePath, profileName)

		_, err := os.Stat(installConfigDirPath)

		if err != nil {
			return "", fmt.Errorf("Site: getProfileType: blueprint profile install config directory (%s) not found", installConfigDirPath)
		}
	}

	var profileType string

	// Try to find an install-config yaml file of some sort
	err = filepath.Walk(installConfigDirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Not interested in directories, so just keep walking
		if info.IsDir() {
			return nil
		}

		if strings.Contains(info.Name(), "install-config") {
			filename, _ := filepath.Abs(path)
			installConfigFile, err := ioutil.ReadFile(filename)

			if err != nil {
				return err
			}

			// Empty file is useless, so just keep walking
			if len(installConfigFile) == 0 {
				return nil
			}

			var installConfig map[string]interface{}

			err = yaml.Unmarshal(installConfigFile, &installConfig)

			if err != nil {
				return err
			}

			// Check unmarshalled YAML for a "platform" map
			if platformIntf, ok := installConfig["platform"].(map[interface{}]interface{}); ok {
				// "platform" map found, so check for certain keys
				for key := range platformIntf {
					switch key.(string) {
					case "aws":
						profileType = "aws"
						break
					case "gcp":
						profileType = "gcp"
						break
					case "libvirt":
						profileType = "libvirt"
						break
					case "none":
						profileType = "baremetal"
						break
					}
				}
			}

			// If we found a profile type, return io.EOF error to force walk to break
			// (we'll catch this below and treat it as a non-error)
			if profileType != "" {
				return io.EOF
			}
		}

		return nil
	})

	// io.EOF error indicates success, but anything else is an actual error
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("Site: getProfileType: error walking blueprint profile install-config directory: %s", err)
	}

	// If we found a profile type, return it
	if profileType != "" {
		return profileType, nil
	}

	// Warn that we were unable to find a profile type
	log.Println("WARNING: Site: getProfileType: unable to determine site profile type")

	return "", nil
}

func (s Site) prepareHostForAutomation(profileName string, requirements map[string]string) error {
	if s.buildPath == "" || s.siteName == "" {
		return errors.New("Site: prepareHostForAutomation: build path and/or site name missing")
	}

	// Clear any existing automation folders
	automationManifests := fmt.Sprintf("%s/%s/automation", s.buildPath, s.siteName)
	automationDestination := fmt.Sprintf("%s/%s/baremetal_automation", s.buildPath, s.siteName)

	os.RemoveAll(automationManifests)
	os.RemoveAll(automationDestination)

	// Get an automated deployment object
	automatedDeployment, err := s.getAutomatedDeployment()

	if err != nil {
		// If automation isn't supported for this profile type, it's not a fatal error in
		// this context, since this function is just trying to prepare the host for potential
		// automation (and is not called in the context of an explicit automation request)
		if strings.Contains(err.Error(), "automation not supported") || strings.Contains(err.Error(), "automated deployment not supported") {
			return nil
		}

		// Anything else should be treated as an error
		return err
	}

	// If automatedDeployment is nil, then automation isn't required/supported
	// for this particular site, which isn't an error in this context
	if automatedDeployment == nil {
		return nil
	}

	// Tell the automated deployment instance to prepare the host for automation
	return automatedDeployment.PrepareAutomation(requirements)
}

func (s Site) finalizeHostForAutomation(profileName string) error {
	if s.buildPath == "" || s.siteName == "" {
		return errors.New("Site: finalizeHostForAutomation: build path and/or site name missing")
	}

	// Get an automated deployment object
	automatedDeployment, err := s.getAutomatedDeployment()

	if err != nil {
		// If automation isn't supported for this profile type, it's not a fatal error in
		// this context, since this function is just trying to finalize the host for potential
		// automation (and is not called in the context of an explicit automation request)
		if strings.Contains(err.Error(), "automation not supported") || strings.Contains(err.Error(), "automated deployment not supported") {
			return nil
		}

		// Anything else should be treated as an error
		return err
	}

	// If automatedDeployment is nil, then automation isn't required/supported
	// for this particular site, which isn't an error in this context
	if automatedDeployment == nil {
		return nil
	}

	// Tell the automated deployment instance to prepare the host for automation
	return automatedDeployment.FinalizeAutomationPreparation()
}
