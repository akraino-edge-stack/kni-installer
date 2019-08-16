package utils

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"time"

	"gopkg.in/yaml.v2"
)

// utility to validate pre-requisites for deploying
func ValidateRequirements(buildPath string, siteName string) {
	// check for pull-secret.json
	if _, err := os.Stat(fmt.Sprintf("%s/pull-secret.json", buildPath)); os.IsNotExist(err) {
		log.Fatal(fmt.Sprintf("Error, no valid pull-secret.json found in %s", buildPath))
		os.Exit(1)
	}

	// check for ssh key , and generate if it does not exist
	if _, err := os.Stat(fmt.Sprintf("%s/id_rsa.pub", buildPath)); os.IsNotExist(err) {
		log.Println(fmt.Sprintf("No SSH public key (id_rsa.pub) found in %s. Generating keypair.", buildPath))

		var envVars []string
		ExecuteCommand("", envVars, true, true, "/bin/bash", "-c", fmt.Sprintf("ssh-keygen -b 2048 -q -N '' -f %s/id_rsa -C user@example.com", buildPath))
	}

	// check if requirements folder exist
	requirementsFolder := fmt.Sprintf("%s/%s/requirements", buildPath, siteName)
	if _, err := os.Stat(requirementsFolder); os.IsNotExist(err) {
		log.Fatal(fmt.Sprintf("Error, requirements folder not found in %s", requirementsFolder))
		os.Exit(1)
	}

}

// utility to get all kustomize dependencies before applying them
func PrepareKustomize(kubectlBinary string, kustomizePath string, kubeconfigPath string) {
	kustomizationContent, err := ioutil.ReadFile(fmt.Sprintf("%s/kustomization.yaml", kustomizePath))
	if err != nil {
		log.Println(fmt.Sprintf("Error reading kustomization content: %s", err))
		os.Exit(1)
	}
	var kustomizationContentObj map[interface{}]interface{}
	err = yaml.Unmarshal(kustomizationContent, &kustomizationContentObj)

	// check if we have patchesjson entry
	var patchesOutput [][]byte
	if jsonPatches, ok := kustomizationContentObj["patchesJson6902"]; ok {
		jsonList := reflect.ValueOf(jsonPatches)
		var targetPatch map[interface{}]interface{}
		var kindPatch interface{}
		var namePatch interface{}
		var namespacePatch interface{}
		var groupPatch interface{}
		var namespaceContent string

		for i := 0; i < jsonList.Len(); i++ {
			currentPatch := jsonList.Index(i).Interface().(map[interface{}]interface{})
			targetPatch, ok = currentPatch["target"].(map[interface{}]interface{})
			if !ok {
				log.Fatal("Error parsing json patch, target not found")
				os.Exit(1)
			}
			if kindPatch, ok = targetPatch["kind"]; !ok {
				log.Fatal("Error parsing json patch, kind not found")
				os.Exit(1)
			}
			if namePatch, ok = targetPatch["name"]; !ok {
				log.Fatal("Error parsing json patch, name not found")
				os.Exit(1)
			}
			if namespacePatch, ok = targetPatch["namespace"]; !ok {
				namespaceContent = ""
			} else {
				namespaceContent = fmt.Sprintf("--namespace %s", namespacePatch)
			}

			if groupPatch, ok = targetPatch["group"]; ok {
				kindPatch = fmt.Sprintf("%s.%s", kindPatch, groupPatch)
			}

			// we have the signature of the patch, let's get the content
			envVars := []string{fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath)}
			finalCommand := fmt.Sprintf("%s get %s/%s -o yaml %s", kubectlBinary, kindPatch, namePatch, namespaceContent)
			out, err := ExecuteCommand("", envVars, false, false, "/bin/bash", "-c", finalCommand)

			if len(err) > 0 {
				log.Fatal(fmt.Sprintf("Error extracting content from %s/%s", kindPatch, namePatch))
				os.Exit(1)
			}

			// if there is output, append to contents
			if out != nil {
				patchesOutput = append(patchesOutput, out)
			}
		}
	}

	// if patchesOutput has content, create an entry in resources to inject the patches bit
	if len(patchesOutput) > 0 {
		resourcesPath := fmt.Sprintf("%s/patches_objects.yaml", kustomizePath)
		os.Remove(resourcesPath)
		f, err := os.Create(resourcesPath)
		defer f.Close()
		if err != nil {
			log.Fatal(fmt.Sprintf("Error creating patches file: %s", resourcesPath))
			os.Exit(1)
		}

		for _, patch := range patchesOutput {
			f.WriteString("---\n")
			f.Write(patch)
		}

		// add the new resource to the yaml
		if resources, ok := kustomizationContentObj["resources"]; ok {
			// if entry does not exist, append it
			found := false
			for _, resourceValue := range resources.([]interface{}) {
				if resourceValue == resourcesPath {
					found = true
					break
				}
			}
			if !found {
				resources = append(resources.([]interface{}), resourcesPath)
				kustomizationContentObj["resources"] = resources
			}
			final, err := yaml.Marshal(&kustomizationContentObj)
			if err != nil {
				log.Fatal("Error manipulating kustomization file")
				os.Exit(1)
			}
			// overwrite the original kustomization file
			err = ioutil.WriteFile(fmt.Sprintf("%s/kustomization.yaml", kustomizePath), final, 0644)
			if err != nil {
				log.Fatal(fmt.Sprintf("Error writing final kustomization file: %s", err))
				os.Exit(1)
			}
		}
	}

	//var basesOutput [][]byte
	if bases, ok := kustomizationContentObj["bases"]; ok {
		for _, baseValue := range bases.([]interface{}) {
			// convert to an absolute path
			absoluteBaseValue := path.Join(kustomizePath, baseValue.(string))

			// recursively call prepare kustomize
			PrepareKustomize(kubectlBinary, absoluteBaseValue, kubeconfigPath)
		}
	}
}

// utility to apply kustomize on a given directory
func ApplyKustomize(kustomizeBinary string, kustomizePath string) []byte {
	// retrieve executable path to inject env var
	ex, err := os.Executable()
	if err != nil {
		log.Fatal("Error retrieving the current running path")
		os.Exit(1)
	}
	exPath := filepath.Dir(ex)

	envVars := []string{fmt.Sprintf("XDG_CONFIG_HOME=%s/plugins", exPath)}
	out, _ := ExecuteCommand("", envVars, true, false, kustomizeBinary, "build", "--enable_alpha_plugins", "--reorder", "none", kustomizePath)

	return out
}

// utility to apply kubectl for a given output
func ApplyKubectl(kubectlBinary string, kubectlContent []byte, kubeconfigPath string) {
	// write content to be applied to temporary file
	tmpFile, err := ioutil.TempFile(os.TempDir(), "kubectl-")
	if err != nil {
		log.Fatal(fmt.Sprintf("Cannot create temporary file: %s", err))
		os.Exit(1)
	}
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write(kubectlContent)
	if err != nil {
		log.Fatal(fmt.Sprintf("Error writing kubectl file: %s", err))
		os.Exit(1)
	}

	envVars := []string{fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath)}
	for i := 1; i <= 10; i++ {
		_, err := ExecuteCommand("", envVars, false, true, kubectlBinary, "apply", "-f", tmpFile.Name())

		if err == nil {
			// it is ok, stop the loop
			break
		} else {
			log.Println(string(err))
			// sleep and retry
			time.Sleep(60 * time.Second)
		}
	}
}

// utility to execute a command and show the stdout and stderr output
func ExecuteCommand(directory string, envVars []string, failFatal bool, showOutput bool, name string, arg ...string) ([]byte, []byte) {
	cmd := exec.Command(name, arg...)

	// set additional modifiers
	if directory != "" {
		cmd.Dir = directory
	}
	cmd.Env = os.Environ()
	for _, envVar := range envVars {
		cmd.Env = append(cmd.Env, envVar)

	}
	var outb, errb bytes.Buffer
	cmd.Stdout = &outb
	cmd.Stderr = &errb
	err := cmd.Run()

	// show output of command
	if showOutput {
		log.Println(outb.String())
	}

	if err != nil {
		if failFatal {
			log.Fatal(fmt.Sprintf("Error applying command: %s - %s", err, errb.String()))
			os.Exit(1)
		} else {
			log.Println(fmt.Sprintf("Error applying command: %s - %s", err, errb.String()))
		}
	}
	return outb.Bytes(), errb.Bytes()
}
