package manifests

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"gopkg.in/yaml.v2"
)

// Generates an unique identifier based on the GKV (from K8s) + Name
func GetGKVN(manifestObj map[interface{}]interface{}) string {

	// retrieves version, and defaults to ~G/~V if not
	version, ok := manifestObj["apiVersion"]
	if !ok {
		version = "~G/~V"
	}

	kind, ok := manifestObj["kind"]
	if !ok {
		kind = "~K"
	}

	var metadata map[interface{}]interface{}
	var name string

	metadata, ok = manifestObj["metadata"].(map[interface{}]interface{})
	if ok {
		if ok {
			name, ok = metadata["name"].(string)
			if !ok {
				name = "~N"
			}
		} else {
			name = "~N"
		}
	} else {
		name = "~N"
	}

	if !strings.Contains(version.(string), "/") {
		version = fmt.Sprintf("~G/%s", version)
	}

	// prepare the final key
	GVKN := fmt.Sprintf("%s/%s|%s", version, kind, name)
	return GVKN
}

// we have a list of items, we need to split and get their individual gvkn
func GetNestedManifestsWithGVKN(manifestObj map[interface{}]interface{}) map[string]map[interface{}]interface{} {
	var items []interface{}
	var parsedItem map[interface{}]interface{}
	var GVKN string
	GVKNS := make(map[string]map[interface{}]interface{})

	items = manifestObj["items"].([]interface{})
	for _, item := range items {
		parsedItem = item.(map[interface{}]interface{})
		GVKN = GetGKVN(parsedItem)
		if len(GVKN) > 0 {
			GVKNS[GVKN] = parsedItem
		}
	}

	return GVKNS
}

// given a gvkn, gets a name from it
func NameFromGVKN(GVKN string) string {
	items := strings.Split(GVKN, "|")
	subItems := strings.Split(items[0], "/")
	name := fmt.Sprintf("%s-%s", subItems[2], items[1])
	if subItems[0] != "~G" {
		name = fmt.Sprintf("%s-%s", subItems[0], name)
	}
	return strings.ToLower(name)
}

// utility to merge manifests
func MergeManifests(content string, siteBuildPath string) {
	manifests := strings.Split(content, "\n---\n")
	kustomizeManifests := make(map[string]map[interface{}]interface{})

	// first split all manifests and unmarshall into objects
	for _, manifest := range manifests {
		var manifestObj map[interface{}]interface{}

		err := yaml.Unmarshal([]byte(manifest), &manifestObj)
		if err != nil {
			log.Println(fmt.Sprintf("Error parsing manifest: %s", err))
			os.Exit(1)
		}
		// add to the list of manifests with the generated key
		GVKN := GetGKVN(manifestObj)
		if GVKN == "~G/v1/List|~N" {
			nestedManifests := GetNestedManifestsWithGVKN(manifestObj)
			for k, v := range nestedManifests {
				kustomizeManifests[k] = v
			}
		} else {
			kustomizeManifests[GVKN] = manifestObj
		}
	}

	// now read all the manifests that have been generated by installer
	processedManifests := make(map[string]string)
	filepath.Walk(fmt.Sprintf("%s/blueprint/base/00_cluster", siteBuildPath), func(path string, info os.FileInfo, err error) error {
		if err == nil {
			// check if it is a file ending with yml/yaml and it is inside openshift or manifests directory
			if !info.IsDir() && (strings.Contains(path, "/openshift/") || strings.Contains(path, "/manifests/")) && (strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
				// read file content and unmarshal it
				manifestContent, err := ioutil.ReadFile(path)
				if err != nil {
					log.Println(fmt.Sprintf("Error reading manifest content: %s", err))
					os.Exit(1)
				}
				var manifestContentObj map[interface{}]interface{}
				err = yaml.Unmarshal(manifestContent, &manifestContentObj)
				if err != nil {
					log.Println(fmt.Sprintf("Error parsing manifest: %s", err))
					os.Exit(1)
				}

				GVKN := GetGKVN(manifestContentObj)
				var walkedManifests map[string]map[interface{}]interface{}
				if GVKN == "~G/v1/List|~N" {
					walkedManifests = GetNestedManifestsWithGVKN(manifestContentObj)
					for k, _ := range walkedManifests {
						processedManifests[k] = ""
					}
				} else {
					walkedManifests = make(map[string]map[interface{}]interface{})
					walkedManifests[GVKN] = manifestContentObj
					processedManifests[GVKN] = ""
				}

				// now compare each content with the ones from kustomize
				counter := 0
				for k, v := range walkedManifests {
					kustomizedContent, ok := kustomizeManifests[k]
					if ok {
						if !reflect.DeepEqual(kustomizedContent, v) {
							// do a backup of the original file
							if _, err := os.Stat(path); err == nil {
								err = os.Rename(path, fmt.Sprintf("%s.orig", path))
							}

							kustomizedString, err := yaml.Marshal(kustomizedContent)
							if err != nil {
								log.Fatal(fmt.Sprintf("Error marshaling kustomized content: %s", err))
								os.Exit(1)
							}

							if len(walkedManifests) == 1 {
								// just rewrite with the original name
								err = ioutil.WriteFile(path, kustomizedString, 0644)
								if err != nil {
									log.Fatal(fmt.Sprintf("Error writing new manifest content: %s", err))
									os.Exit(1)
								}
							} else {
								// rewrite with a prefix
								newPath := fmt.Sprintf("%02d_%s", counter, path)
								err = ioutil.WriteFile(newPath, kustomizedString, 0644)
								if err != nil {
									log.Fatal(fmt.Sprintf("Error writing new manifest content: %s", err))
									os.Exit(1)
								}
								counter = counter + 1
							}
						}
					}
				}

			}
		} else {
			log.Println(fmt.Sprintf("Error walking on manifests directory: %s", err))
			os.Exit(1)
		}
		return nil
	})

	// now find manifests not yet in assets dir and write them out
	counter := 0
	for k, v := range kustomizeManifests {
		_, ok := processedManifests[k]
		if !ok {
			// the manifest is not there, add it
			manifestName := fmt.Sprintf("99_%04d_%s.yaml", counter, NameFromGVKN(k))
			log.Println(fmt.Sprintf("Blueprint added manifests %s, writing to %s", k, manifestName))

			newPath := fmt.Sprintf("%s/blueprint/base/00_cluster/manifests/%s", siteBuildPath, manifestName)

			// marshal the file to write
			kustomizedString, err := yaml.Marshal(v)
			if err != nil {
				log.Fatal(fmt.Sprintf("Error marshing manifest: %s", err))
				os.Exit(1)
			}
			err = ioutil.WriteFile(newPath, kustomizedString, 0644)
			if err != nil {
				log.Fatal(fmt.Sprintf("Error writing manifest: %s", err))
				os.Exit(1)
			}
			counter = counter + 1

		}
	}

	// finally, move content to final manifests
	os.RemoveAll(fmt.Sprintf("%s/final_manifests", siteBuildPath))
	err := os.Rename(fmt.Sprintf("%s/blueprint/base/00_cluster", siteBuildPath), fmt.Sprintf("%s/final_manifests", siteBuildPath))
	if err != nil {
		log.Fatal(fmt.Sprintf("Error moving to final manifests folder: %s", err))
		os.Exit(1)
	} else {
		log.Println(fmt.Sprintf("*** Manifest generation finished. You can run now: %s/requirements/openshift-install create cluster --dir=%s/final_manifests to create the site cluster ***", siteBuildPath, siteBuildPath))
		log.Println(fmt.Sprintf("In order to destroy the cluster you can run:  %s/requirements/openshift-install destroy cluster --dir %s/final_manifests", siteBuildPath, siteBuildPath))
	}

}
