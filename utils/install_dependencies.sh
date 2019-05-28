#!/bin/bash

URL=$(curl https://api.github.com/repos/kubernetes-sigs/kustomize/releases/latest |grep browser_download | grep linux | cut -d '"' -f 4)
sudo -E curl -L $URL -o /usr/local/bin/kustomize
sudo chmod a+x /usr/local/bin/kustomize

wget -A "openshift-client-linux-4*\.tar\.gz" -r -np -nc -nd -l1 --no-check-certificate -e robots=off https://mirror.openshift.com/pub/openshift-v4/clients/ocp/latest/ -P /tmp/
sudo tar -xvf /tmp/openshift-client-linux-4*.tar.gz -C /usr/local/bin/ oc
rm -f /tmp/openshift-client-linux-4*.tar.gz

echo "Install virtctl to manage Kubevirt based VMs"
export KUBEVIRT_VERSION=$(curl https://github.com/kubevirt/kubevirt/releases/latest | awk -F"tag/" '{print $2}' | cut -d \" -f 1)
curl -L -o /usr/local/bin/virtctl https://github.com/kubevirt/kubevirt/releases/download/${KUBEVIRT_VERSION}/virtctl-${KUBEVIRT_VERSION}-linux-amd64
chmod +x /usr/local/bin/virtctl
