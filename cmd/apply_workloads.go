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
	"log"
	"os"

	"gerrit.akraino.org/kni/installer/pkg/site"
	"github.com/spf13/cobra"
)

// applyWorkloadsCmd represents the apply_workloads command
var applyWorkloadsCmd = &cobra.Command{
	Use:              "apply_workloads siteName [--build_path=<local_build_path>]",
	Short:            "Command to apply the workloads after deploying a site",
	Long:             ``,
	TraverseChildren: true,
	Run: func(cmd *cobra.Command, args []string) {
		// retrieve config values and start fetching
		var siteName string
		if len(args) == 0 {
			log.Fatal("Please specify site name as first argument")
			os.Exit(1)
		} else {
			siteName = args[0]
		}

		buildPath, _ := cmd.Flags().GetString("build_path")
		if len(buildPath) == 0 {
			// will generate a temporary directory
			buildPath = fmt.Sprintf("%s/.kni", os.Getenv("HOME"))
		}

		kubeconfig, _ := cmd.Flags().GetString("kubeconfig")
		if len(kubeconfig) == 0 {
			// set to default value
			kubeconfig = fmt.Sprintf("%s/%s/final_manifests/auth/kubeconfig", buildPath, siteName)
		} else if kubeconfig == "local" {
			kubeconfig = ""
		}

		// define a site object and proceed with applying workloads
		s := site.NewWithName(siteName, buildPath)
		s.ApplyWorkloads(kubeconfig)
	},
}

func init() {
	rootCmd.AddCommand(applyWorkloadsCmd)

	applyWorkloadsCmd.Flags().StringP("build_path", "", "", "Directory to use as build path. If that not exists, the installer will generate a default directory")
	applyWorkloadsCmd.Flags().StringP("kubeconfig", "", "", "Path to kubeconfig file. By default it will be the one generated with prepare_manifests. If set to 'local', no kubeconfig will be used and it will assume running on local cluster")

}
