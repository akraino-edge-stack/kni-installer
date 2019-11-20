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

SITE_NAME="testing.baremetal.edge-sites.net"
MATCHBOX_ENDPOINT="http://172.22.0.1:8080"

rm -rf $HOME/.kni/$SITE_NAME
pushd $HOME/go/src/gerrit.akraino.org/kni/installer
./knictl fetch_requirements file://${WORKSPACE}/$SITE_NAME
./knictl prepare_manifests $SITE_NAME
popd

pushd $HOME/go/src/gerrit.akraino.org/kni/installer
./knictl deploy_masters $SITE_NAME
$HOME/.kni/$SITE_NAME/requirements/openshift-install wait-for bootstrap-complete  --dir $HOME/.kni/$SITE_NAME/baremetal_automation/ocp/
popd

pushd $HOME/.kni/$SITE_NAME/baremetal_automation/kickstart
bash add_kickstart_for_centos.sh
cp centos-worker-kickstart.cfg $HOME/.kni/$SITE_NAME/baremetal_automation/matchbox-data/var/lib/matchbox/assets
popd

mount -o loop /opt/images/CentOS-7-x86_64-DVD-1810.iso /mnt/
cp -ar /mnt/. $HOME/.kni/$SITE_NAME/baremetal_automation/matchbox-data/var/lib/matchbox/assets/centos7/
chmod -R 755 $HOME/.kni/$SITE_NAME/baremetal_automation/matchbox-data/var/lib/matchbox/assets/centos7/
umount /mnt

# replace the settings_upi.env settings
sed -i "s#export KUBECONFIG_PATH=.*#export KUBECONFIG_PATH=$HOME/.kni/$SITE_NAME/baremetal_automation/ocp/auth/kubeconfig#g" /root/settings_upi.env
sed -i "s#export OS_INSTALL_ENDPOINT=.*#export OS_INSTALL_ENDPOINT=$MATCHBOX_ENDPOINT/assets/centos7#g" /root/settings_upi.env

pushd $HOME/go/src/gerrit.akraino.org/kni/installer
./knictl deploy_workers $SITE_NAME

# just sleep for some time, and workers should be up
sleep 20m
popd

echo "Cluster successfully deployed! Start applying workloads"

rm -rf $HOME/.kni/$SITE_NAME/baremetal_automation/build/openshift-patches/99-ifcfg-*.yaml

pushd $HOME/go/src/gerrit.akraino.org/kni/installer
./knictl apply_workloads $SITE_NAME --kubeconfig $HOME/.kni/$SITE_NAME/baremetal_automation/ocp/auth/kubeconfig 

STATUS=$?
popd

if [ $STATUS -ne 0 ]; then
    echo "Error applying workloads to baremetal"
    exit 1
fi

# now destroy the cluster
pushd $HOME/go/src/gerrit.akraino.org/kni/installer
./knictl destroy_cluster $SITE_NAME
popd
