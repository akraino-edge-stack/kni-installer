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
	"os"

	"gerrit.akraino.org/kni/installer/pkg/site"
	"github.com/spf13/cobra"
)

// fetchRequirementsCmd represents the fetch_requirements command
var fetchRequirementsCmd = &cobra.Command{
	Use:              "fetch_requirements",
	Short:            "Command to fetch the requirements needed for a site",
	Long:             ``,
	TraverseChildren: true,
	Run: func(cmd *cobra.Command, args []string) {
		// retrieve config values and start fetching
		siteRepo, _ := cmd.Flags().GetString("site")
		buildPath, _ := cmd.Flags().GetString("build_path")
		if len(buildPath) == 0 {
			// will generate a temporary directory
			buildPath = fmt.Sprintf("%s/.kni", os.Getenv("HOME"))
		}

		// define a site object and proceed with requirements fetch
		s := site.New(siteRepo, buildPath)
		s.DownloadSite()
		s.FetchRequirements()
	},
}

func init() {
	rootCmd.AddCommand(fetchRequirementsCmd)

	fetchRequirementsCmd.Flags().StringP("site", "", "", "Url/path for site repository. Can be in any go-getter compatible format")
	fetchRequirementsCmd.MarkFlagRequired("site")
	fetchRequirementsCmd.Flags().StringP("build_path", "", "", "Directory to use as build path. If that not exists, the installer will generate a default directory")

}
