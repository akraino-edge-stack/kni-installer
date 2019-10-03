package automation

import (
	"errors"
	"fmt"
)

type AutomatedDeploymentInterface interface {
	PrepareBastion() error // Prepare host for automation
	DeployMasters() error  // Deploy cluster masters
	DeployWorkers() error  // Deploy cluster workers
}

var (
	// If we find that different profile types (libvirt, aws, etc) that we add
	// in the future require different constructor parameter count/types, then
	// we can redo the approach here
	automatedDeploymentConstructors map[string]func(string, string, string) (AutomatedDeploymentInterface, error)
)

func init() {
	// Add new automation profile types here
	automatedDeploymentConstructors = map[string]func(string, string, string) (AutomatedDeploymentInterface, error){}
	automatedDeploymentConstructors["baremetal"] = newBaremetal
}

// Generates a new automation deployment instance
func New(profileType string, siteBuildPath string, siteName string, siteRepo string) (AutomatedDeploymentInterface, error) {
	if siteBuildPath == "" {
		return nil, errors.New("AutomatedDeployment: New: site build path not provided")
	}

	if siteName == "" {
		return nil, errors.New("AutomatedDeployment: New: site name not provided")
	}

	if siteRepo == "" {
		return nil, errors.New("AutomatedDeployment: New: site repo not provided")
	}

	constructor := automatedDeploymentConstructors[profileType]

	// If no constructor available, then automation is not available for this profile type
	if constructor == nil {
		return nil, fmt.Errorf("AutomatedDeployment: New: automation not supported for profile type '%s", profileType)
	}

	// Constructors should return nil as the AutomatedDeploymentInterface if automation is
	// not supported for the particular site
	automatedDeployment, err := constructor(siteBuildPath, siteName, siteRepo)

	if err != nil {
		return nil, err
	}

	// It's up to the caller to decide how to respond to nil and non-nil automatedDeployment
	return automatedDeployment, nil
}
