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

	"gerrit.akraino.org/kni/installer/pkg/manifests"
	"gerrit.akraino.org/kni/installer/pkg/requirements"
	"gerrit.akraino.org/kni/installer/pkg/utils"
	getter "github.com/hashicorp/go-getter"
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
		log.Println(fmt.Sprintf("Cloning the site repository from %s", s.siteRepo))
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
				log.Fatal(fmt.Sprintf("Error cloning site repository: %s", err))
			}
		}
	} else {
		log.Fatal(fmt.Sprintf("Site repository does not exist for the site %s", s.siteName))
		os.Exit(1)
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
			log.Fatal(fmt.Sprintf("Error reading profile file: %s", err))
			os.Exit(1)
		}

		profileSettings := &map[string][]interface{}{}
		err = yaml.Unmarshal(yamlContent, &profileSettings)
		if err != nil {
			log.Fatal(fmt.Sprintf("Error parsing profile yaml file: %s", err))
			os.Exit(1)
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
		log.Fatal(fmt.Sprintf("File %s does not exist, exiting", profileFile))
		os.Exit(1)
	}

	return "", "", ""
}

// using the downloaded site content, fetches (and builds) the specified requirements
func (s Site) FetchRequirements(individualRequirements []string) {
	log.Println(fmt.Sprintf("Downloading requirements for %s", s.siteName))
	sitePath := fmt.Sprintf("%s/%s", s.buildPath, s.siteName)

	// searches for file containing the profile of the blueprint
	profileName, profileLayerPath, _ := s.GetProfileFromSite()

	profileBuildPath := fmt.Sprintf("%s/%s", sitePath, profileName)
	log.Println(fmt.Sprintf("Downloading profile repo from %s into %s", profileLayerPath, profileBuildPath))
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
			log.Fatal(fmt.Sprintf("Error cloning profile repository: %s", err))
		}
	}

	// read yaml from requirements and start fetching the bits
	requirementsFile := fmt.Sprintf("%s/requirements.yaml", profileBuildPath)
	file, err := os.Open(requirementsFile)
	if err != nil {
		log.Fatal("Error reading requirements file")
		os.Exit(1)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		requirementsLine := scanner.Text()

		// requirements is composed of binary and source
		requirementsBits := strings.SplitN(strings.TrimSpace(requirementsLine), ":", 2)
		binaryName := strings.TrimSpace(requirementsBits[0])

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
				log.Println(fmt.Sprintf("Binary %s not found in list, skipping", binaryName))
				continue
			}
		}
		r := requirements.New(binaryName, strings.TrimSpace(requirementsBits[1]), fmt.Sprintf("%s/requirements", sitePath))
		r.FetchRequirement()
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
		log.Fatal(fmt.Sprintf("Error reading site config file: %s", err))
		os.Exit(1)
	}

	// parse the yaml and check for key value
	err = yaml.Unmarshal(b, &configFileObj)
	if err != nil {
		log.Println(fmt.Sprintf("Error parsing manifest: %s", err))
		os.Exit(1)
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
		log.Fatal(fmt.Sprintf("Error writing profile.env file: %s", err))
		os.Exit(1)
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

	log.Println(fmt.Sprintf("Downloading blueprint repo from %s", downloadRepo))
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
			log.Fatal(fmt.Sprintf("Error cloning profile repository: %s", err))
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
					log.Fatal(fmt.Sprintf("Error opening kustomization file: %s", err))
					os.Exit(1)
				}

				newKustomization := strings.Replace(string(readKustomization), absoluteBlueprintRepo, "../../../", -1)
				if profileRef != "" {
					newKustomization = strings.Replace(newKustomization, fmt.Sprintf("?ref=%s", profileRef), "", -1)
				}

				err = ioutil.WriteFile(path, []byte(newKustomization), 0)
				if err != nil {
					log.Fatal(fmt.Sprintf("Error writing modified kustomization file: %s", err))
					os.Exit(1)
				}
				return nil
			}

		} else {
			log.Println(fmt.Sprintf("Error walking on site directory: %s", err))
			os.Exit(1)
		}

		return nil
	})
}

// using the downloaded site content, prepares the manifests for it
func (s Site) PrepareManifests() {
	sitePath := fmt.Sprintf("%s/%s", s.buildPath, s.siteName)
	log.Println(fmt.Sprintf("Preparing manifests for %s", s.siteName))

	// do the initial validation of pre-requisites
	utils.ValidateRequirements(s.buildPath, s.siteName)
	binariesPath := fmt.Sprintf("%s/requirements", sitePath)

	// retrieve profile path and clone the repo
	_, profileLayerPath, profileRef := s.GetProfileFromSite()
	s.DownloadRepo(sitePath, profileLayerPath, profileRef)

	// generate openshift-install manifests based on phase 00_install-config
	assetsPath := fmt.Sprintf("%s/generated_assets", sitePath)
	os.RemoveAll(assetsPath)
	os.Mkdir(assetsPath, 0755)

	out := utils.ApplyKustomize(fmt.Sprintf("%s/kustomize", binariesPath), fmt.Sprintf("%s/blueprint/sites/site/00_install-config", sitePath))
	// check if we have any content and write to the target file
	if len(out) > 0 {
		err := ioutil.WriteFile(fmt.Sprintf("%s/install-config.yaml", assetsPath), out, 0644)
		if err != nil {
			log.Fatal(fmt.Sprintf("Error writing final install-config file: %s", err))
			os.Exit(1)
		}

	} else {
		log.Fatal("Error, kustomize did not return any content")
		os.Exit(1)
	}

	// now generate the manifests
	var envVars []string
	utils.ExecuteCommand("", envVars, true, true, fmt.Sprintf("%s/openshift-install", binariesPath), "create", "manifests", fmt.Sprintf("--dir=%s", assetsPath), "--log-level", "debug")
	// iterate over all the generated files and create a kustomization file
	f, err := os.Create(fmt.Sprintf("%s/kustomization.yaml", assetsPath))
	if err != nil {
		log.Fatal(fmt.Sprintf("Error creating kustomization file: %s", err))
		os.Exit(1)
	}
	defer f.Close()

	_, err = f.WriteString("resources:\n")
	if err != nil {
		log.Fatal(fmt.Sprintf("Error writing kustomization file: %s", err))
		os.Exit(1)
	}

	filePatterns := []string{fmt.Sprintf("%s/manifests/*.yaml", assetsPath), fmt.Sprintf("%s/manifests/*.yml", assetsPath), fmt.Sprintf("%s/openshift/*.yaml", assetsPath)}
	for _, filePattern := range filePatterns {
		files, err := filepath.Glob(filePattern)
		if err != nil {
			log.Fatal(fmt.Sprintf("Error reading manifest files: %s", err))
			os.Exit(1)
		}

		// iterate over each file, remove the absolute path and write it
		for _, fileName := range files {
			strippedName := strings.TrimPrefix(fileName, fmt.Sprintf("%s/", assetsPath))
			_, err := f.WriteString(fmt.Sprintf("- %s\n", strippedName))
			if err != nil {
				log.Fatal(fmt.Sprintf("Error writing kustomization file: %s", err))
				os.Exit(1)
			}
		}
	}

	// move the content of the generated site to blueprint base
	os.Rename(fmt.Sprintf("%s/generated_assets/", sitePath), fmt.Sprintf("%s/blueprint/base/00_cluster/", sitePath))

	// apply kustomize on cluster-mods
	out = utils.ApplyKustomize(fmt.Sprintf("%s/kustomize", binariesPath), fmt.Sprintf("%s/blueprint/sites/site/01_cluster-mods", sitePath))
	if len(out) > 0 {
		// now apply modifications on the manifests
		manifests.MergeManifests(string(out), sitePath)

	} else {
		log.Fatal("Error, kustomize did not return any content")
		os.Exit(1)
	}

}

// using the site contents, applies the workloads on it
func (s Site) ApplyWorkloads(kubeconfigFile string) {
	siteBuildPath := fmt.Sprintf("%s/%s", s.buildPath, s.siteName)

	// if we have kubeconfig, validate that exists
	if len(kubeconfigFile) > 0 {
		if _, err := os.Stat(kubeconfigFile); err != nil {
			log.Fatal(fmt.Sprintf("Error: kubeconfig file %s does not exist", kubeconfigFile))
			os.Exit(1)
		}
	}
	binariesPath := fmt.Sprintf("%s/requirements", siteBuildPath)
	ocPath := fmt.Sprintf("%s/oc", binariesPath)
	kustomizePath := fmt.Sprintf("%s/kustomize", binariesPath)

	// retrieve profile path and clone the repo
	_, profileLayerPath, profileRef := s.GetProfileFromSite()
	s.DownloadRepo(siteBuildPath, profileLayerPath, profileRef)

	addonsPath := fmt.Sprintf("%s/blueprint/sites/site/02_cluster-addons", siteBuildPath)
	log.Println(fmt.Sprintf("Applying workloads from %s", addonsPath))
	utils.PrepareKustomize(ocPath, addonsPath, kubeconfigFile)
	out := utils.ApplyKustomize(kustomizePath, addonsPath)
	if string(out) != "" {
		// now we can apply it
		utils.ApplyOc(fmt.Sprintf("%s/oc", binariesPath), out, kubeconfigFile)
	} else {
		log.Println(fmt.Sprintf("No manifests found for %s", addonsPath))
	}
	servicesPath := fmt.Sprintf("%s/blueprint/sites/site/03_services", siteBuildPath)
	log.Println(fmt.Sprintf("Applying workloads from %s", servicesPath))
	utils.PrepareKustomize(ocPath, servicesPath, kubeconfigFile)
	out = utils.ApplyKustomize(kustomizePath, servicesPath)
	if string(out) != "" {
		// now we can apply it
		utils.ApplyOc(ocPath, out, kubeconfigFile)
	} else {
		log.Println(fmt.Sprintf("No manifests found for %s", servicesPath))
	}
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
		return "", errors.New("Site: getProfileType: blueprint profile install config directory not found")
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
