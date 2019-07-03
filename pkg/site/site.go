package site

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"

	"gerrit.akraino.org/kni/installer/pkg/requirements"
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
	client := &getter.Client{Src: s.siteRepo, Dst: siteBuildPath, Mode: getter.ClientModeAny}
	err := client.Get()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error cloning site repository: %s", err))
	}

}

// using the downloaded site content, fetches (and builds) the specified requirements
func (s Site) FetchRequirements() {
	log.Println(fmt.Sprintf("Downloading requirements for %s", s.siteName))
	siteBuildPath := fmt.Sprintf("%s/%s", s.buildPath, s.siteName)

	// searches for file containing the profile of the blueprint
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

		profileBuildPath := fmt.Sprintf("%s/%s", siteBuildPath, profileName)
		log.Println(fmt.Sprintf("Downloading profile repo from %s into %s", profilePath, profileBuildPath))
		client := &getter.Client{Src: profilePath, Dst: profileBuildPath, Mode: getter.ClientModeAny}
		err = client.Get()
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

	} else if os.IsNotExist(err) {
		log.Fatal(fmt.Sprintf("File %s does not exist, exiting", profileFile))
		os.Exit(1)
	}
}
