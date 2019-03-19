package generator

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	getter "github.com/hashicorp/go-getter"
	"gopkg.in/yaml.v2"
)

// Generator : Structure that contains the settings needed for generation
type Generator struct {
	baseRepo               string
	basePath               string
	installerPath          string
	secretsRepo            string
	siteRepo               string
	settingsPath           string
	buildPath              string
	masterMemoryMB         string
	sshKeyPath             string
	secrets                map[string]string
	manifestKeys           map[string]string
	manifestKeyDefinitions map[string][][]string
}

// New constructor for the generator
func New(baseRepo string, basePath string, installerPath string, secretsRepo string, siteRepo string, settingsPath string, buildPath string, masterMemoryMB string, sshKeyPath string) Generator {
	manifestKeyDefinitions := map[string][][]string{
		"manifests/cvo-overrides.yaml":                         [][]string{[]string{"spec", "clusterID"}},
		"manifests/kube-system-configmap-etcd-serving-ca.yaml": [][]string{[]string{"data", "ca-bundle.crt"}},
		"manifests/kube-system-configmap-root-ca.yaml":         [][]string{[]string{"data", "ca.crt"}},
		"manifests/kube-system-secret-etcd-client.yaml":        [][]string{[]string{"data", "tls.crt"}, []string{"data", "tls.key"}},
		"manifests/machine-config-server-tls.yaml":             [][]string{[]string{"data", "tls.crt"}, []string{"data", "tls.key"}},
		"manifests/machine-config-server-tls-secret.yaml":      [][]string{[]string{"data", "tls.crt"}, []string{"data", "tls.key"}},
		"manifests/cluster-dns-02-config.yml":                  [][]string{[]string{"spec", "publicZone", "id"}},
		"manifests/pull.json":                                  [][]string{[]string{"data", ".dockerconfigjson"}},
		"openshift/99_cloud-creds-secret.yaml":                 [][]string{[]string{"data", "aws_access_key_id"}, []string{"data", "aws_secret_access_key"}},
		"openshift/99_kubeadmin-password-secret.yaml":          [][]string{[]string{"data", "kubeadmin"}},
	}

	g := Generator{baseRepo, basePath, installerPath, secretsRepo, siteRepo, settingsPath, buildPath, masterMemoryMB, sshKeyPath,
		make(map[string]string), make(map[string]string), manifestKeyDefinitions}
	return g
}

// DownloadArtifacts is a method for downloading all the initial artifacts
func (g Generator) DownloadArtifacts() {
	// Download installer for openshift
	log.Println("Downloading openshift-install binary")
	binaryPath := fmt.Sprintf("%s/openshift-install", g.buildPath)
	client := &getter.Client{Src: g.installerPath, Dst: binaryPath}
	err := client.Get()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error downloading openshift-install binary: %s", err))
		os.Exit(1)
	}
	os.Chmod(binaryPath, 0777)

	// Download the credentials repo
	log.Println("Download secrets repo")
	secretsPath := fmt.Sprintf("%s/secrets", g.buildPath)

	// Retrieve private key and b64encode it, if secrets is not local
	finalURL := ""
	if !strings.HasPrefix(g.secretsRepo, "file://") {
		priv, err := ioutil.ReadFile(g.sshKeyPath)
		if err != nil {
			log.Fatal(fmt.Sprintf("Error reading secret key: %s", err))
			os.Exit(1)
		}
		sEnc := base64.StdEncoding.EncodeToString(priv)
		finalURL = fmt.Sprintf("%s?sshkey=%s", g.secretsRepo, sEnc)
	} else {
		finalURL = g.secretsRepo
	}
	client = &getter.Client{Src: finalURL, Dst: secretsPath, Mode: getter.ClientModeAny}
	err = client.Get()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error downloading secrets repo: %s", err))
		os.Exit(1)
	}
	os.Chmod(secretsPath, 0700)

	// Clone the base repository with base manifests
	log.Println("Cloning the base repository with base manifests")
	baseBuildPath := fmt.Sprintf("%s/base_manifests", g.buildPath)
	client = &getter.Client{Src: g.baseRepo, Dst: baseBuildPath, Mode: getter.ClientModeAny}
	err = client.Get()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error cloning base repository with base manifests: %s", err))
		os.Exit(1)
	}

	// Clone the site repository with settings.yaml
	log.Println("Cloning the site repository with settings")
	siteBuildPath := fmt.Sprintf("%s/site", g.buildPath)
	client = &getter.Client{Src: g.siteRepo, Dst: siteBuildPath, Mode: getter.ClientModeAny}
	err = client.Get()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error cloning site repository: %s", err))
	}
}

// ReadSecretFiles will traverse secrets directory and read content
func (g Generator) ReadSecretFiles(path string, info os.FileInfo, err error) error {
	var matches = map[string]string{"coreos-pull-secret": "pullSecret",
		"ssh-pub-key": "SSHKey", "aws-access-key-id": "aws-access-key-id",
		"aws-secret-access-key": "aws-secret-access-key"}

	if info.IsDir() && info.Name() == ".git" {
		return filepath.SkipDir
	}

	if err != nil {
		log.Fatal(fmt.Sprintf("Error traversing file: %s", err))
		os.Exit(1)
	}

	if !info.IsDir() {
		data, err := ioutil.ReadFile(path)
		if err != nil {
			log.Fatal(fmt.Sprintf("Error reading file content: %s", err))
			os.Exit(1)
		}
		g.secrets[matches[info.Name()]] = strings.Trim(string(data), "\n")
	}
	return nil
}

// GenerateInstallConfig generates the initial config.yaml
func (g Generator) GenerateInstallConfig() {
	// Read install-config.yaml on the given path and parse it
	manifestsPath := fmt.Sprintf("%s/base_manifests/%s", g.buildPath, g.basePath)
	installPath := fmt.Sprintf("%s/install-config.yaml.go", manifestsPath)

	t, err := template.New("install-config.yaml.go").ParseFiles(installPath)
	if err != nil {
		log.Fatal(fmt.Sprintf("Error reading install file: %s", err))
		os.Exit(1)
	}

	// parse settings file
	settingsPath := fmt.Sprintf("%s/site/%s", g.buildPath, g.settingsPath)
	yamlContent, err := ioutil.ReadFile(settingsPath)
	if err != nil {
		log.Fatal(fmt.Sprintf("Error reading settings file: %s", err))
		os.Exit(1)
	}

	siteSettings := &map[string]map[string]interface{}{}
	err = yaml.Unmarshal(yamlContent, &siteSettings)
	if err != nil {
		log.Fatal(fmt.Sprintf("Error parsing settings yaml file: %s", err))
		os.Exit(1)
	}
	parsedSettings := (*siteSettings)["settings"]

	// Read secrets
	secretsPath := fmt.Sprintf("%s/secrets", g.buildPath)
	ln, err := filepath.EvalSymlinks(secretsPath)
	if err != nil {
		log.Fatal(fmt.Sprintf("Error evaluating symlinks: %s", err))
		os.Exit(1)
	}
	if len(ln) > 0 {
		// we need to traverse that instead of the given path
		secretsPath = ln
	}
	err = filepath.Walk(secretsPath, g.ReadSecretFiles)

	// Prepare the final file to write the template
	workingPath := fmt.Sprintf("%s/working_manifests", g.buildPath)
	os.RemoveAll(workingPath)
	os.MkdirAll(workingPath, 0775)

	f, err := os.Create(fmt.Sprintf("%s/working_manifests/install-config.yaml", g.buildPath))
	if err != nil {
		log.Fatal(fmt.Sprintf("Error opening the install file: %s", err))
		os.Exit(1)
	}

	// Prepare the vars to be executed in the template
	var settings = make(map[string]string)
	var mandatorySettings = map[string]string{
		"baseDomain":          "string",
		"clusterName":         "string",
		"clusterCIDR":         "string",
		"clusterSubnetLength": "int",
		"machineCIDR":         "string",
		"serviceCIDR":         "string",
		"SDNType":             "string",
	}
	for key, value := range mandatorySettings {
		if _, ok := parsedSettings[key]; ok {
			if value == "int" {
				settings[key] = fmt.Sprintf("%d", parsedSettings[key].(int))
			} else {
				settings[key] = parsedSettings[key].(string)
			}
		}
	}
	// Settings depending on provider
	providerSettings := [2]string{"libvirtURI", "AWSRegion"}
	for _, element := range providerSettings {
		if _, ok := parsedSettings[element]; ok {
			settings[element] = parsedSettings[element].(string)
		}
	}

	// Merge with secrets dictionary
	for k, v := range g.secrets {
		settings[k] = v
	}
	err = t.Execute(f, settings)

	if err != nil {
		log.Fatal(fmt.Sprintf("Error parsing template: %s", err))
	}

}

// GenerateCredentials create the AWS credentials file if needed
func (g Generator) GenerateCredentials() {
	AWSKey, ok1 := g.secrets["aws-access-key-id"]
	AWSSecret, ok2 := g.secrets["aws-secret-access-key"]

	if ok1 && ok2 {
		log.Println("Generating AWS credentials")
		// generate aws creds file
		settings := make(map[string]string)
		settings["AWSKey"] = AWSKey
		settings["AWSSecret"] = AWSSecret

		// Create AWS credentials directory, or overwrite it
		AWSPath := fmt.Sprintf("%s/.aws", os.Getenv("HOME"))
		os.RemoveAll(AWSPath)
		os.MkdirAll(AWSPath, 0700)

		f, err := os.Create(fmt.Sprintf("%s/credentials", AWSPath))
		if err != nil {
			log.Fatal(fmt.Sprintf("Error opening the install file: %s", err))
			os.Exit(1)
		}
		os.Chmod(fmt.Sprintf("%s/credentials", AWSPath), 0600)

		t, err := template.New("aws").Parse("[default]\naws_access_key_id={{.AWSKey}}\naws_secret_access_key={{.AWSSecret}}")
		err = t.Execute(f, settings)
	} else {
		log.Println("No secrets provided, skipping credentials creation")
	}
}

// CreateManifests creates the initial manifests for the cluster
func (g Generator) CreateManifests() {
	log.Println("Creating manifests")
	cmd := exec.Command("./openshift-install", "create", "manifests", "--dir", "working_manifests")
	cmd.Dir = g.buildPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error creating manifests: %s - %s", err, string(out)))
		os.Exit(1)
	}
}

func findValue(what map[interface{}]interface{}, wantedPath []string) (wantedValue string) {
	return search(what, nil, wantedPath)
}

func search(what map[interface{}]interface{}, previous []string, wantedPath []string) (wantedValue string) {
	for key, value := range what {
		if value2, ok := value.(map[interface{}]interface{}); ok {
			var myprevious []string

			if previous == nil {
				myprevious = []string{}
			} else {
				myprevious = previous
			}

			myprevious = append(myprevious, key.(string))
			match := true

			for i, prevKey := range myprevious {
				if prevKey != wantedPath[i] {
					match = false
					break
				}
			}

			if match {
				wantedValue = search(value2, myprevious, wantedPath)
			}

			if wantedValue != "" {
				return wantedValue
			}
		} else {
			if key.(string) == wantedPath[len(wantedPath)-1] {
				return fmt.Sprintf("%v", value)
			}
		}
	}

	return ""
}

// ExtractKeysFromManifests will extract the needed keys for future deployments
func (g Generator) ExtractKeysFromManifests() {
	// The Kubernetes Go client (nested within the OpenShift Go client)
	// automatically registers its types in scheme.Scheme, however the
	// additional OpenShift types must be registered manually.  AddToScheme
	// registers the API group types (e.g. route.openshift.io/v1, Route) only.

	log.Println("Extracting keys")

	// iterate over all key definitions
	for file, keys := range g.manifestKeyDefinitions {
		log.Println(fmt.Sprintf("Parsing file: %s", file))
		log.Println(keys)
		// first check if the file exists
		manifestFile := fmt.Sprintf("%s/working_manifests/%s", g.buildPath, file)
		if _, err := os.Stat(manifestFile); err == nil {
			// parse the manifest file
			yamlContent, err := ioutil.ReadFile(manifestFile)
			if err != nil {
				log.Fatal(fmt.Sprintf("Error reading manifest file: %s", err))
				os.Exit(1)
			}

			content := map[interface{}]interface{}{}

			err = yaml.Unmarshal(yamlContent, &content)

			for _, value := range keys {
				log.Println(fmt.Sprintf("Parsing key %s", value))
				seekedValue := findValue(content, value)
				log.Println(seekedValue)
			}
		}
	}
	log.Println(g.manifestKeys)
}

// DeployCluster starts deployment of the cluster
func (g Generator) DeployCluster() {
	log.Println("Deploying cluster")
	cmd := exec.Command("./openshift-install", "create", "cluster", "--dir", "working_manifests")
	cmd.Dir = g.buildPath

	if len(g.masterMemoryMB) > 0 {
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, fmt.Sprintf("TF_VAR_libvirt_master_memory=%s", g.masterMemoryMB))
	}

	var stdBuffer bytes.Buffer
	mw := io.MultiWriter(os.Stdout, &stdBuffer)
	cmd.Stdout = mw
	cmd.Stderr = mw

	err := cmd.Run()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error creating cluster: %s - %s", err, stdBuffer.String()))
		os.Exit(1)
	}
	log.Println(stdBuffer.String())
}

// GenerateManifests generates the manifests with the parsed values
func (g Generator) GenerateManifests() {
	// First download the needed artifacts
	g.DownloadArtifacts()

	// Generate install-config.yaml
	g.GenerateInstallConfig()

	// Generate credentials for AWS if needed
	g.GenerateCredentials()

	// Create manifests
	g.CreateManifests()

	// Extract needed keys from the manifests
	g.ExtractKeysFromManifests()

	// Deploy cluster
	g.DeployCluster()
}
