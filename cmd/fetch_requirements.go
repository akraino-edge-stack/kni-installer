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
	"strings"

	"gerrit.akraino.org/kni/installer/pkg/site"
	"github.com/spf13/cobra"
)

// fetchRequirementsCmd represents the fetch_requirements command
var fetchRequirementsCmd = &cobra.Command{
	Use:              "fetch_requirements siteRepo [--build_path=<local_build_path>]",
	Short:            "Command to fetch the requirements needed for a site",
	Long:             ``,
	TraverseChildren: true,
	Run: func(cmd *cobra.Command, args []string) {
		// we need to have at least site as first argument
		var siteRepo string
		if len(args) == 0 {
			log.Fatalln("Please specify site repository as first argument")
		} else {
			siteRepo = args[0]
		}
		// retrieve config values and start fetching
		buildPath, _ := cmd.Flags().GetString("build_path")
		if len(buildPath) == 0 {
			// will generate a temporary directory
			buildPath = fmt.Sprintf("%s/.kni", os.Getenv("HOME"))
		}

		// check if we have a requirements list specified
		var requirements []string
		requirementsList, _ := cmd.Flags().GetString("requirements")
		if len(requirementsList) > 0 {
			// strip list in array
			requirements = strings.Split(requirementsList, ",")
		}

		// define a site object and proceed with requirements fetch
		s := site.New(siteRepo, buildPath)
		s.DownloadSite()
		s.FetchRequirements(requirements)
	},
}

func init() {
	rootCmd.AddCommand(fetchRequirementsCmd)

	fetchRequirementsCmd.Flags().StringP("build_path", "", "", "Directory to use as build path. If that not exists, the installer will generate a default directory")
	fetchRequirementsCmd.Flags().StringP("requirements", "", "", "Individual requirements list. It needs to be a list of requirements separated by commas. If not supplied, all requirements will be downloaded")
}
