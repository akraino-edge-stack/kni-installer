package site

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
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

// given a site repo, downloads the content and places into buildPath
func (s Site) DownloadSite() {
	// Clone the site repository
	log.Println(fmt.Sprintf("Cloning the site repository from %s", s.siteRepo))
	siteBuildPath := fmt.Sprintf("%s/%s/site", s.buildPath, s.siteName)
	os.RemoveAll(siteBuildPath)
	client := &getter.Client{Src: s.siteRepo, Dst: siteBuildPath, Mode: getter.ClientModeAny}
	err := client.Get()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error cloning site repository: %s", err))
	}

}

// retrieves the given profile used in a site
func (s Site) GetProfileFromSite() (string, string) {
	siteBuildPath := fmt.Sprintf("%s/%s", s.buildPath, s.siteName)

	profileFile := fmt.Sprintf("%s/site/00_install-config/kustomization.yaml", siteBuildPath)

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
		profileBits := strings.Split(profileRepo, "/")
		profileName := profileBits[len(profileBits)-2]
		profilePath := strings.TrimSuffix(profileRepo, profileBits[len(profileBits)-1])

		return profileName, profilePath
	} else if os.IsNotExist(err) {
		log.Fatal(fmt.Sprintf("File %s does not exist, exiting", profileFile))
		os.Exit(1)
	}

	return "", ""
}

// using the downloaded site content, fetches (and builds) the specified requirements
func (s Site) FetchRequirements() {
	log.Println(fmt.Sprintf("Downloading requirements for %s", s.siteName))
	siteBuildPath := fmt.Sprintf("%s/%s", s.buildPath, s.siteName)

	// searches for file containing the profile of the blueprint
	profileName, profilePath := s.GetProfileFromSite()

	profileBuildPath := fmt.Sprintf("%s/%s", siteBuildPath, profileName)
	log.Println(fmt.Sprintf("Downloading profile repo from %s into %s", profilePath, profileBuildPath))
	client := &getter.Client{Src: profilePath, Dst: profileBuildPath, Mode: getter.ClientModeAny}
	err := client.Get()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error cloning profile repository: %s", err))
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
		r := requirements.New(strings.TrimSpace(requirementsBits[0]), strings.TrimSpace(requirementsBits[1]), fmt.Sprintf("%s/requirements", siteBuildPath))
		r.FetchRequirement()
	}

	// remove profile folder
	os.RemoveAll(profileBuildPath)

}

// using the downloaded site content, prepares the manifests for it
func (s Site) PrepareManifests() {
	siteBuildPath := fmt.Sprintf("%s/%s", s.buildPath, s.siteName)
	log.Println(fmt.Sprintf("Preparing manifests for %s", s.siteName))

	// do the initial validation of pre-requisites
	utils.ValidateRequirements(s.buildPath, s.siteName)
	binariesPath := fmt.Sprintf("%s/requirements", siteBuildPath)

	// retrieve profile path and clone the repo
	_, profilePath := s.GetProfileFromSite()
	indexGit := strings.LastIndex(profilePath, "//")
	var profileRepo string
	var absoluteProfileRepo string
	if indexGit == -1 {
		profileRepo = profilePath
		absoluteProfileRepo = profilePath
	} else {
		profileRepo = profilePath[0:indexGit]
		absoluteProfileRepo = profilePath[0:(indexGit + 2)]
	}

	log.Println(fmt.Sprintf("Downloading profile repo from %s", profileRepo))
	blueprintDir := fmt.Sprintf("%s/blueprint", siteBuildPath)
	os.RemoveAll(blueprintDir)
	client := &getter.Client{Src: profileRepo, Dst: blueprintDir, Mode: getter.ClientModeAny}
	err := client.Get()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error cloning profile repository: %s", err))
	}

	// and now move site inside the sites folder, replacing the absolute references to relative
	os.Rename(fmt.Sprintf("%s/site", siteBuildPath), fmt.Sprintf("%s/blueprint/sites/site", siteBuildPath))
	err = filepath.Walk(fmt.Sprintf("%s/blueprint/sites/site", siteBuildPath), func(path string, info os.FileInfo, err error) error {
		if err == nil {
			if info.Name() == "kustomization.yaml" {
				readKustomization, err := ioutil.ReadFile(path)
				if err != nil {
					log.Fatal(fmt.Sprintf("Error opening kustomization file: %s", err))
					os.Exit(1)
				}
				newKustomization := strings.Replace(string(readKustomization), absoluteProfileRepo, "../../../", -1)

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

	// generate openshift-install manifests based on phase 00_install-config
	finalSitePath := fmt.Sprintf("%s/generated_site", siteBuildPath)
	os.RemoveAll(finalSitePath)
	os.Mkdir(finalSitePath, 0755)

	out := utils.ApplyKustomize(fmt.Sprintf("%s/kustomize", binariesPath), fmt.Sprintf("%s/blueprint/sites/site/00_install-config", siteBuildPath))
	// check if we have any content and write to the target file
	if len(out) > 0 {
		err := ioutil.WriteFile(fmt.Sprintf("%s/install-config.yaml", finalSitePath), out, 0644)
		if err != nil {
			log.Fatal(fmt.Sprintf("Error writing final install-config file: %s", err))
			os.Exit(1)
		}

	} else {
		log.Fatal("Error, kustomize did not return any content")
		os.Exit(1)
	}

	// now generate the manifests
	cmd := exec.Command(fmt.Sprintf("%s/openshift-install", binariesPath), "create", "manifests", fmt.Sprintf("--dir=%s", finalSitePath), "--log-level", "debug")
	err = cmd.Run()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error creating manifests: %s", err))
		os.Exit(1)
	}

	// iterate over all the generated files and create a kustomization file
	f, err := os.Create(fmt.Sprintf("%s/kustomization.yaml", finalSitePath))
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

	filePatterns := []string{fmt.Sprintf("%s/manifests/*.yaml", finalSitePath), fmt.Sprintf("%s/manifests/*.yml", finalSitePath), fmt.Sprintf("%s/openshift/*.yaml", finalSitePath)}
	for _, filePattern := range filePatterns {
		files, err := filepath.Glob(filePattern)
		if err != nil {
			log.Fatal(fmt.Sprintf("Error reading manifest files: %s", err))
			os.Exit(1)
		}

		// iterate over each file, remove the absolute path and write it
		for _, fileName := range files {
			strippedName := strings.TrimPrefix(fileName, fmt.Sprintf("%s/", finalSitePath))
			_, err := f.WriteString(fmt.Sprintf("- %s\n", strippedName))
			if err != nil {
				log.Fatal(fmt.Sprintf("Error writing kustomization file: %s", err))
				os.Exit(1)
			}
		}
	}

	// move the content of the generated site to blueprint base
	os.Rename(fmt.Sprintf("%s/generated_site/", siteBuildPath), fmt.Sprintf("%s/blueprint/base/00_cluster/", siteBuildPath))

	// apply kustomize on cluster-mods
	out = utils.ApplyKustomize(fmt.Sprintf("%s/kustomize", binariesPath), fmt.Sprintf("%s/blueprint/sites/site/01_cluster-mods", siteBuildPath))
	if len(out) > 0 {
		// now apply modifications on the manifests
		manifests.MergeManifests(string(out), siteBuildPath)

	} else {
		log.Fatal("Error, kustomize did not return any content")
		os.Exit(1)
	}

}
