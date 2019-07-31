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

function approve_certs() {
    # sleep for the first 10 min
    sleep 600

    # temporary fix, autoapprove certificates on background
    while /bin/true; do
        export KUBECONFIG=$HOME/.kni/${SITE_NAME}/final_manifests/auth/kubeconfig
        oc get csr | grep worker | grep Pending | awk '{print $1}' | xargs -n 1 oc adm certificate approve || true
        sleep 60
    done
}

# move the blueprint to an inner directory
mkdir ${WORKSPACE}/blueprint-pae
mv base profiles sites tools ${WORKSPACE}/blueprint-pae/

# clone installer in the right directory
sudo rm -rf ${WORKSPACE}/${KNI_PATH}
mkdir -p ${WORKSPACE}/${KNI_PATH}
pushd ${WORKSPACE}/${KNI_PATH}/
git clone https://gerrit.akraino.org/r/kni/installer
pushd installer

# first build kni installer
make build 2>&1 | tee ${WORKSPACE}/build.log

# do a host preparation and cleanup
bash utils/prep_host.sh virt.edge-sites.net
wget https://raw.githubusercontent.com/openshift/installer/master/scripts/maintenance/virsh-cleanup.sh
chmod a+x ./virsh-cleanup.sh
sudo -E bash -c "yes Y | ./virsh-cleanup.sh"

# add the right credentials to kni
mkdir $HOME/.kni || true
cp $WORKSPACE/akraino-secrets/coreos-pull-secret $HOME/.kni/pull-secret.json || true
cp $HOME/.ssh/id_rsa.pub $HOME/.kni/id_rsa.pub || true

# start the workflow
sudo rm -rf /$HOME/.kni/${SITE_NAME}/final_manifests || true
./knictl fetch_requirements file://${WORKSPACE}/blueprint-pae//sites/${SITE_NAME} 2>&1 | tee ${WORKSPACE}/libvirt_requirements.log
./knictl prepare_manifests ${SITE_NAME} 2>&1 | tee ${WORKSPACE}/libvirt_manifests.log

# now run the cluster
source $HOME/.kni/${SITE_NAME}/profile.env
approve_certs &
FUNCTION_PID=$!
sudo -E $HOME/.kni/${SITE_NAME}/requirements/openshift-install create cluster --dir=/$HOME/.kni/${SITE_NAME}/final_manifests 2>&1 | tee ${WORKSPACE}/libvirt_deploy.log
STATUS=$?
kill $FUNCTION_PID || true

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
popd

exit $STATUS
