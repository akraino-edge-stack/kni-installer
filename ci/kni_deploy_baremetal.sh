#!/bin/bash
#
# Copyright (c) 2019 Red Hat
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#        http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e -u -x -o pipefail

SITE_NAME="${SITE_NAME:-testing.baremetal.edge-sites.net}"
MATCHBOX_ENDPOINT="${MATCHBOX_ENDPOINT:-http://172.22.0.1:8080}"
UPI_NAME="${UPI_NAME:-testing}"
UPI_DOMAIN="${UPI_DOMAIN:-baremetal.edge-sites.net}"
LANG="en_US.UTF-8"
LC_ALL="en_US.UTF-8"
PRESERVE_CLUSTER="${PRESERVE_CLUSTER:-true}"

# Stop the VMs before the containers because the container filesystems
# appear in the VM filesystem mounts.
wget https://raw.githubusercontent.com/openshift/installer/master/scripts/maintenance/virsh-cleanup.sh
chmod a+x ./virsh-cleanup.sh
sudo -E bash -c "yes Y | ./virsh-cleanup.sh"

# Stop the containers so they can be removed and their names re-used later.
podman stop kni-dnsmasq-prov || /bin/true
podman stop kni-dnsmasq-bm || /bin/true
podman stop kni-haproxy || /bin/true
podman stop kni-coredns || /bin/true
podman stop kni-matchbox || /bin/true

# Removed the stopped containers.
podman rm kni-dnsmasq-prov || /bin/true
podman rm kni-dnsmasq-bm || /bin/true
podman rm kni-haproxy || /bin/true
podman rm kni-coredns || /bin/true
podman rm kni-matchbox || /bin/true

# In case a container removal happened while a VM was still running,
# it will no longer appear in CLI output as a container that podman
# knows about, but the storage will remain and an entry will still be
# present in containers.json which will prevent creating a container
# with the same name.  This should clean up that situation, but
# otherwise is not normally necessary.
podman rm -f --storage kni-dnsmasq-prov || /bin/true
podman rm -f --storage kni-dnsmasq-bm || /bin/true
podman rm -f --storage kni-haproxy || /bin/true
podman rm -f --storage kni-coredns || /bin/true
podman rm -f --storage kni-matchbox || /bin/true

rm -rf $HOME/.kni/$SITE_NAME || true
pushd $HOME/go/src/gerrit.akraino.org/kni/installer
./bin/knictl fetch_requirements file://${WORKSPACE}/kni-blueprint-pae/sites/$SITE_NAME
./bin/knictl prepare_manifests $SITE_NAME
popd

pushd $HOME/go/src/gerrit.akraino.org/kni/installer
./bin/knictl deploy_masters $SITE_NAME

# just sleep for some time, for masters to be up
sleep 20m

NUM_READY=0
set +e
while [[ "$NUM_READY" -lt 1 ]]; do
    READY_NODES=$(KUBECONFIG=$HOME/.kni/$SITE_NAME/baremetal_automation/ocp/auth/kubeconfig $HOME/.kni/$SITE_NAME/requirements/oc get nodes || true)
    NUM_READY=$(echo $READY_NODES | grep " Ready " | wc -l )
    sleep 1m
done
set -e
popd

pushd $HOME/go/src/gerrit.akraino.org/kni/installer
./bin/knictl deploy_workers $SITE_NAME

# destroy bootstrap node
virsh destroy ${UPI_NAME}-bootstrap

# just sleep for some time, and workers should be up
sleep 20m
popd

echo "Cluster successfully deployed! Start applying workloads"

rm -rf $HOME/.kni/$SITE_NAME/baremetal_automation/build/openshift-patches/99-ifcfg-*.yaml

pushd $HOME/go/src/gerrit.akraino.org/kni/installer
./bin/knictl apply_workloads $SITE_NAME --kubeconfig $HOME/.kni/$SITE_NAME/baremetal_automation/ocp/auth/kubeconfig

STATUS=$?
popd

if [ $STATUS -ne 0 ]; then
    echo "Error applying workloads to baremetal"
    exit 1
fi

if [ -z "${PRESERVE_CLUSTER}" ]; then
  # now destroy the cluster
  pushd $HOME/go/src/gerrit.akraino.org/kni/installer
  ./bin/knictl destroy_cluster $SITE_NAME
  popd
fi
