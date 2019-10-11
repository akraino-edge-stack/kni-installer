package automation

import (
	"errors"
	"fmt"
)

// ProfileType, SiteBuildPath and SiteName are always needed (and are always
// checked by the automation package's New function).  Anything else added here
// is the responsibility of the automation interface implementation to validate
type AutomatedDeploymentParams struct {
	ProfileType   string
	SiteBuildPath string
	SiteName      string
	SiteRepo      string
}

type AutomatedDeploymentInterface interface {
	PrepareBastion() error // Prepare host for automation
	DeployMasters() error  // Deploy cluster masters
	DeployWorkers() error  // Deploy cluster workers
	DestroyCluster() error // Destroy the cluster
}

var (
	// If we find that different profile types (libvirt, aws, etc) that we add
	// in the future require different constructor parameter count/types, then
	// we can redo the approach here
	automatedDeploymentConstructors map[string]func(AutomatedDeploymentParams) (AutomatedDeploymentInterface, error)
)

func init() {
	// Add new automation profile types here
	automatedDeploymentConstructors = map[string]func(AutomatedDeploymentParams) (AutomatedDeploymentInterface, error){}
	automatedDeploymentConstructors["baremetal"] = newBaremetal
}

// Generates a new automation deployment instance
func New(params AutomatedDeploymentParams) (AutomatedDeploymentInterface, error) {
	// SiteBuildPath is always needed
	if params.SiteBuildPath == "" {
		return nil, errors.New("AutomatedDeployment: New: site build path not provided")
	}

	// SiteName is always needed
	if params.SiteName == "" {
		return nil, errors.New("AutomatedDeployment: New: site name not provided")
	}

	// ProfileType is always needed
	constructor := automatedDeploymentConstructors[params.ProfileType]

	// If no constructor available, then automation is not available for this profile type
	if constructor == nil {
		return nil, fmt.Errorf("AutomatedDeployment: New: automation not supported for profile type '%s", params.ProfileType)
	}

	// Constructors should return nil as the AutomatedDeploymentInterface if automation is
	// not supported for the particular site
	automatedDeployment, err := constructor(params)

	if err != nil {
		return nil, err
	}

	// It's up to the caller to decide how to respond to nil and non-nil automatedDeployment
	return automatedDeployment, nil
}
