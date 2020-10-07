// Copyright © 2019 Red Hat <abays@redhat.com>
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

// deployMastersCmd represents the deploy_masters command
var deployMastersCmd = &cobra.Command{
	Use:              "deploy_masters siteName [--build_path=<local_build_path>]",
	Short:            "Command to automate the deployment of the master nodes of a previously-prepared site",
	Long:             ``,
	TraverseChildren: true,
	Run: func(cmd *cobra.Command, args []string) {
		// retrieve config values and start fetching
		var siteName string
		if len(args) == 0 {
			log.Fatalln("Please specify site name as first argument")
		} else {
			siteName = args[0]
		}

		buildPath, _ := cmd.Flags().GetString("build_path")
		if len(buildPath) == 0 {
			// will generate a temporary directory
			buildPath = fmt.Sprintf("%s/.kni", os.Getenv("HOME"))
		}

		// This command is used after fetch_requirements and prepare_manifests,
		// so the site directory should be available on disk already (if not,
		// s.AutomateMastersDeployment will error-out appropriately)
		s := site.NewWithName(siteName, buildPath)
		s.AutomateMastersDeployment()
	},
}

func init() {
	rootCmd.AddCommand(deployMastersCmd)

	deployMastersCmd.Flags().StringP("build_path", "", "", "Directory to use as build path. If that doesn't exist, the installer will generate a default directory")
}
