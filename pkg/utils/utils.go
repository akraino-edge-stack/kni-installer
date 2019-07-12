package utils

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
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

		cmd := exec.Command("ssh-keygen", "-b", "2048", "-f", fmt.Sprintf("%s/id_rsa", buildPath), "-C", "user@example.com", "-q", "-N", "\"\"")
		err = cmd.Run()
		if err != nil {
			log.Fatal(fmt.Sprintf("Error generating ssh keypair: %s", err))
			os.Exit(1)
		}
	}

	// check if requirements folder exist
	requirementsFolder := fmt.Sprintf("%s/%s/requirements", buildPath, siteName)
	if _, err := os.Stat(requirementsFolder); os.IsNotExist(err) {
		log.Fatal(fmt.Sprintf("Error, requirements folder not found in %s", requirementsFolder))
		os.Exit(1)
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

	cmd := exec.Command(kustomizeBinary, "build", "--enable_alpha_plugins", "--reorder", "none", kustomizePath)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("XDG_CONFIG_HOME=%s/plugins", exPath))
	out, err := cmd.Output()

	if err != nil {
		log.Fatal(fmt.Sprintf("Error kustomizing manifests for %s: %s", kustomizePath, err))
		os.Exit(1)
	}

	return out
}

// utility to apply kubectl for a given output
func ApplyKubectl(kubectlBinary string, kubectlContent []byte, kubeconfigPath string) {
	var out []byte
	for i := 1; i <= 10; i++ {
		cmd := exec.Command(kubectlBinary, "apply", "-f", "-")

		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, fmt.Sprintf("KUBECONFIG_PATH=%s", kubeconfigPath))

		// add output to stdin
		stdin, err := cmd.StdinPipe()
		stdin.Write(kubectlContent)
		stdin.Close()

		out, err = cmd.Output()

		// show output for user to see progress
		log.Println(string(out))

		if err == nil {
			// it is ok, stop the loop
			break
		} else {
			// sleep and retry
			time.Sleep(60 * time.Second)
		}
	}
}
