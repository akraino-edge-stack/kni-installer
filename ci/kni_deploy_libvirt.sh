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

export PATH=$PATH:/usr/local/go/bin:/usr/local/bin
KNI_PATH='src/gerrit.akraino.org/kni/'
SITE_NAME='testing.virt.edge-sites.net'

echo '---> Starting kni installer generation'
export GOPATH=${WORKSPACE}/

# first build kni installer
sudo rm -rf ${WORKSPACE}/${KNI_PATH}
mkdir -p ${WORKSPACE}/${KNI_PATH}/installer
cp -a installer ${WORKSPACE}/${KNI_PATH}/
pushd ${WORKSPACE}/${KNI_PATH}/installer
make build 2>&1 | tee ${WORKSPACE}/build.log

# do a host preparation and cleanup
bash utils/prep_host.sh virt.edge-sites.net
wget https://raw.githubusercontent.com/openshift/installer/master/scripts/maintenance/virsh-cleanup.sh
chmod a+x ./virsh-cleanup.sh
sudo -E bash -c "yes Y | ./virsh-cleanup.sh"

# add the right credentials to kni
mkdir $HOME/.kni || true
cp $HOME/.ssh/id_rsa.pub $HOME/.kni/id_rsa.pub || true

# replace site path with a local ref to the cloned blueprint
BLUEPRINT_PATH="${WORKSPACE}/${GIT_CHECKOUT_DIR}/"
KUSTOMIZATION_FILE=${BLUEPRINT_PATH}/sites/${SITE_NAME}/00_install-config/kustomization.yaml
sed -i "s#- git::https://gerrit.akraino.org/r/kni/${GIT_CHECKOUT_DIR}.git/#- file://${BLUEPRINT_PATH}#g" ${KUSTOMIZATION_FILE}

# start the workflow
sudo rm -rf /$HOME/.kni/${SITE_NAME}/final_manifests || true
./knictl fetch_requirements file://${BLUEPRINT_PATH}/sites/${SITE_NAME} 2>&1 | tee ${WORKSPACE}/libvirt_requirements.log
./knictl prepare_manifests ${SITE_NAME} 2>&1 | tee ${WORKSPACE}/libvirt_manifests.log

# move the binary to /usr/bin, due to create cluster constraints
sudo cp $HOME/.kni/${SITE_NAME}/requirements/openshift-install /usr/bin/openshift-install

# now run the cluster
pushd /usr/bin
source $HOME/.kni/${SITE_NAME}/profile.env
sudo -E /usr/bin/openshift-install create cluster --dir=/$HOME/.kni/${SITE_NAME}/final_manifests 2>&1 | tee ${WORKSPACE}/libvirt_deploy.log
STATUS=$?
popd

# output tfstate
echo "metadata.json for removing cluster"
cat $HOME/.kni/${SITE_NAME}/final_manifests/metadata.json

if [ $STATUS -ne 0 ]; then
    echo "Error deploying in libvirt"
    exit 1
fi

echo "Cluster successfully deployed! Start applying workloads"
./knictl apply_workloads ${SITE_NAME} 2>&1 | tee ${WORKSPACE}/libvirt_workloads.log
STATUS=$?

if [ $STATUS -ne 0 ]; then
    echo "Error applying workloads to libvirt"
    exit 1
fi

popd

exit $STATUS
