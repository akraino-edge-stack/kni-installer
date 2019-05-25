// Copyright © 2019 Red Hat <yroblamo@redhat.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"

	getter "github.com/hashicorp/go-getter"
	"github.com/spf13/cobra"
)

// generateWorkloads will run kustomize and apply the generated workloads on the cluster
func generateWorkloads(siteRepository string, buildPath string, clusterCredentials string) {
	// Clone the site repository
	log.Println("Cloning the site repository")
	siteBuildPath := fmt.Sprintf("%s/site", buildPath)
	client := &getter.Client{Src: siteRepository, Dst: siteBuildPath, Mode: getter.ClientModeAny}
	err := client.Get()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error cloning site repository: %s", err))
	}

	// apply kustomize on the given path
	log.Println("Generating workloads")
	workloadsPath := fmt.Sprintf("%s/workloads", siteBuildPath)
	cmd := exec.Command("kustomize", "build", workloadsPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error calling kustomize: %s - %s", err, string(out)))
		os.Exit(1)
	}
	outfile, err := os.Create(fmt.Sprintf("%s/content.yaml", workloadsPath))
	if err != nil {
		log.Fatal(fmt.Sprintf("Error storing output from kustomize: %s", err))
		os.Exit(1)
	}
	_, err = outfile.WriteString(fmt.Sprintf("%s", out))
	if err != nil {
		log.Fatal(fmt.Sprintf("Error writing kustomize file: %s", err))
	}
	defer outfile.Close()

	// and now apply the content of the file
	log.Println("Applying the generated workloads")
	cmd = exec.Command("oc", "apply", "-f", fmt.Sprintf("%s/content.yaml", workloadsPath))
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("KUBECONFIG=%s", clusterCredentials))
	out, err = cmd.CombinedOutput()
	if err != nil {
		log.Fatal(fmt.Sprintf("Error applying kustomize workloads: %s - %s", err, out))
		os.Exit(1)
	}
	log.Println(fmt.Sprintf("%s", out))
}

// workloadsCmd represents the workloads command
var workloadsCmd = &cobra.Command{
	Use:              "workloads",
	Short:            "Command to apply workloads on a working cluster",
	Long:             ``,
	TraverseChildren: true,
	Run: func(cmd *cobra.Command, args []string) {
		// retrieve config values and start generation
		siteRepository, _ := cmd.Flags().GetString("site_repository")

		// Check if build path exists, create if not
		buildPath, _ := cmd.Flags().GetString("build_path")
		if len(buildPath) == 0 {
			// will generate a temporary directory
			buildPath, _ = ioutil.TempDir("/tmp", "kni")
		} else {
			// remove if exists, recreate
			os.RemoveAll(buildPath)
			os.MkdirAll(buildPath, 0775)
		}

		clusterCredentials, _ := cmd.Flags().GetString("cluster_credentials")

		// start generation process
		generateWorkloads(siteRepository, buildPath, clusterCredentials)
	},
}

func init() {
	rootCmd.AddCommand(workloadsCmd)

	workloadsCmd.Flags().StringP("site_repository", "", "", "Url for the specific site workloads folder")
	workloadsCmd.MarkFlagRequired("site_repository")
	workloadsCmd.Flags().StringP("cluster_credentials", "", "", "The credentials to use to access the cluster")
	workloadsCmd.MarkFlagRequired("cluster_credentials")

}
