## KNI installer

This repository contains the installer for the Akraino KNI deployment. Along with the [https://gerrit.akraino.org/r/#/admin/projects/kni/templates](https://gerrit.akraino.org/r/#/admin/projects/kni/templates) repository, it will allow to deploy Edge sites in a declarative way.

## Dependencies

You will need to create a Red Hat account on [https://www.redhat.com/wapps/ugc/register.html](https://www.redhat.com/wapps/ugc/register.html), then login with your account into [https://cloud.redhat.com/openshift](https://cloud.redhat.com/openshift). This is needed to have download access to the OpenShift installer artifacts.
After that, you will need to get the Pull Secret from
[https://cloud.redhat.com/openshift/install/metal/user-provisioned](https://cloud.redhat.com/openshift/install/metal/user-provisioned) - Copy Pull Secret link.

## How to build

First the `kni-edge-installer` binary needs to be produced. For that you just execute make with the following syntax:

    make build

This will produce the `kni-edge-installer` binary that can be used to deploy a site.

## How to deploy

There is a Makefile on the root directory of this project. In order to deploy
you will need to use the following syntax:

    make deploy CREDENTIALS=<git_private_repo> SETTINGS=<path_to_sample_settings> BASE_REPO=<git_base_manifests_repo> BASE_PATH=<path_in_manifests_repo>

**CREDENTIALS: Content of private repo**

This repository is needed to store private credentials. It is recommended that
you store those credentials on a private repo where only allowed people have
access, where access to the repositories can be controlled by SSH keys.
Each setting is stored in individual files in the repository:

- ssh-pub-key           # public key to be used for SSH access into the nodes
- coreos-pull-secret    # place the file that you created before
- aws-access-key-id     # just for AWS deploy, key id that you created in AWS
- aws-secret-access-key # just for AWS deploy, secret for the key id

The right path to clone a private repo is: git@github.com:repo_user/repo_name.git

You also can use a local directory to store secrets in local deployments.
You should create a directory with a known path, and your directory shall contain the individual files listed before. In that case no SSH key is needed. You can reference you local secrets repo by:

    CREDENTIALS=file://<path_to_secrets_repo>

**BASE_REPO: Repository for the base manifests**

This is the repository where the default manifest templates are stored. There is one specific folder for each blueprint and provider: aws/1-node, libvirt/1-node, etc... This can be any repository with the right templates, but for Akraino it currently defaults to github.com/redhat-nfvpe/kni-edge-base.git

**BASE_PATH: Path inside base manifests**

Once the manifests repository is being cloned, it will contain several folders with all the specific manifests for the blueprint. In order to choose a provider and footprint, the BASE_PATH needs to be specific. Current values are: libvirt/1-node, libvirt/3-node, aws/1-node, aws/3-node

**SITE_REPO: Repository for the site manifests**

Each site can have different manifests and settings. This needs to be a per-site repository where individual settings need to be stored, and where the generated manifests per site will be stored (TODO).

**SETTINGS_PATH: Specific site settings**

Once the site repository is being cloned, it needs to contain a settings.yaml file with the per-site settings. This needs to be the path inside the SITE_REPO where the settings.yaml is contained. A sample settings file can be seen at [https://gerrit.akraino.org/r/gitweb?p=kni/templates.git;a=blob_plain;f=libvirt/sample_settings.yaml;hb=refs/heads/master](https://gerrit.akraino.org/r/gitweb?p=kni/templates.git;a=blob_plain;f=libvirt/sample_settings.yaml;hb=refs/heads/master) for libvirt, and at [https://gerrit.akraino.org/r/gitweb?p=kni/templates.git;a=blob_plain;f=aws/sample_settings.yaml;hb=refs/heads/master](https://gerrit.akraino.org/r/gitweb?p=kni/templates.git;a=blob_plain;f=aws/sample_settings.yaml;hb=refs/heads/master) for AWS. Different settings file need to be created per site.

## How to deploy for AWS

Before starting the deploy, please read the following documentation to prepare
the AWS account properly:
[https://github.com/openshift/installer/blob/master/docs/user/aws/README.md](https://github.com/openshift/installer/blob/master/docs/user/aws/README.md)

There are two different footprints for AWS: 1 master/1 worker, and 3 masters/3 workers. Makefile needs to be called with:

    make deploy CREDENTIALS=git@github.com:<git_user>/akraino-secrets.git SITE_REPO=git::https://gerrit.akraino.org/r/kni/templates.git SETTINGS_PATH=aws/sample_settings.yaml BASE_REPO=git::https://gerrit.akraino.org/r/kni/templates.git BASE_PATH=[aws/1-node|aws/3-node]

The file will look like:

    settings:
      baseDomain: "<base_domain>"
      clusterName: "<cluster_name>"
      clusterCIDR: "10.128.0.0/14"
      clusterSubnetLength: 9
      machineCIDR: "10.0.0.0/16"
      serviceCIDR: "172.30.0.0/16"
      SDNType: "OpenShiftSDN"
      AWSRegion: "<aws_region_to_deploy>"

Where:
- `<base_domain>` is the DNS zone matching with the one created on Route53
- `<cluster_name>` is the name you are going to give to the cluster
- `<aws_region_to_deploy>` is the region where you want your cluster to deploy

SETTINGS can be a path to local file, or an url, will be queried with curl.

The make process will create the needed artifacts and will start the deployment of the specified cluster

Please consider that, for security reasons. AWS has disabled the SSH access to
nodes. If you need to SSH for any reason, please take a look at `Unable to SSH
into Master Nodes` section from the following document:

[https://github.com/openshift/installer/blob/master/docs/user/troubleshooting.md](https://github.com/openshift/installer/blob/master/docs/user/troubleshooting.md)


## How to deploy for Libvirt

First of all, we need to prepare a host in order to configure libvirt, iptables, permissions, etc.
This repository contains a bash script that will prepare the host for you.

    source utils/prep_host.sh

The official documentation that describes these changes can be found in the following link:

[https://github.com/openshift/installer/blob/master/docs/dev/libvirt-howto.md](https://github.com/openshift/installer/blob/master/docs/dev/libvirt-howto.md)

Unfortunately, Libvirt is only for development purposes from the OpenShift perspective, so the binary is not compiled with the libvirt bits by default. The user will have to compile it by his/her own version with libvirt enabled. In order to build a binary, please look at section `Building and consuming your own installer`. As a summary, doing `make binary` will compile openshift-installer binay with libvirt support.

There are two different footprints for libvirt: 1 master/1 worker, and 3 masters/3 workers. Makefile needs to be called with:

    make deploy CREDENTIALS=git@github.com:<git_user>/akraino-secrets.git SITE_REPO=git::https://gerrit.akraino.org/r/kni/templates.git SETTINGS_PATH=libvirt/sample_settings.yaml BASE_REPO=git::https://gerrit.akraino.org/r/kni/templates.git BASE_PATH=[libvirt/1-node|libvirt/3-node] INSTALLER_PATH=file:///${GOPATH}/bin/openshift-install

A sample settings.yaml file has been created specifically for Libvirt targets. It needs to look like:

    settings:
      baseDomain: "<base_domain>"
      clusterName: "<cluster_name>"
      clusterCIDR: "10.128.0.0/14"
      clusterSubnetLength: 9
      machineCIDR: "10.0.0.0/16"
      serviceCIDR: "172.30.0.0/16"
      SDNType: "OpenShiftSDN"
      libvirtURI: "<libvirt_host_ip>"

Where:
- `<base_domain>` is the DNS zone matching with the entry created in /etc/NetworkManager/dnsmasq.d/openshift.conf during the libvirt-howto machine setup. (tt.testing by default)
- `<cluster_name>` is the name you are going to give to the cluster
- `<libvirt_host_ip>` is the host IP where libvirt is configured (i.e. qemu+tcp://192.168.122.1/system)

The rest of the options are exactly the same as in an AWS deployment.

There is a recording of the deployment of a Libvirt blueprint with 1 master and 1 worker. Please, see the following link to watch it:

[https://www.youtube.com/watch?v=3mDb1cba8uU](https://www.youtube.com/watch?v=3mDb1cba8uU)

## How to use the cluster

After the deployment finishes, a `kubeconfig` file will be placed inside
build/auth directory:

    export KUBECONFIG=./build/auth/kubeconfig

Then cluster can be managed with oc. You can get the client on this link
[https://mirror.openshift.com/pub/openshift-v4/clients/ocp/latest/](https://mirror.openshift.com/pub/openshift-v4/clients/ocp/latest/), and check for `openshift-client` files (there are versions for Linux, Mac and Windows). Once downloaded, uncompress it, give it execution permissions and move to the desired path. After that, regular `oc` commands can be executed over the cluster.

## How to destroy the cluster

In order to destroy the running cluster, and clean up environment, just use
`make clean` command.

## Building and consuming your own installer

The openshift-installer binaries are published on [https://mirror.openshift.com/pub/openshift-v4/clients/ocp/latest/](https://mirror.openshift.com/pub/openshift-v4/clients/ocp/latest/).
For faster deploy, you can grab the installer from the upper link. However, there may be situations where you need to compile your own installer (such as the case of libvirt), or when you need a newer version.
You can build the binary following the instructions on [https://github.com/openshift/installer](https://github.com/openshift/installer) - Quick Start section, or you can use the provided target from our project.
The binary can be produced with the following command:

    make binary

It accepts an additional INSTALLER_GIT_TAG parameter that allows to specify the installer version that you want to build. Once built, the `openshift-install` is placed in the build directory. This binary can be copied into $GOPATH/bin for easier use. Once generated, the new installer can be used with an env var:

    export INSTALLER_PATH=http://<url_to_binary>/openshift-install

Or pass it as a parameter to make command.

## Customization: use your own manifests

openshift-installer is also able to produce manifests, that end users can modify
and deploy a cluster with the modified settings. New manifests can be generated
with:

    /path/to/openshift-install create manifests

This will generate a pair of folders: manifests and openshift. Those manifests
can be modified with the desired values. After that this code can be executed to
generate a new cluster based on the modified manifests:

    /path/to/openshift-install create cluster