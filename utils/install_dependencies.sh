#!/bin/bash

URL=$(curl https://api.github.com/repos/kubernetes-sigs/kustomize/releases/latest |grep browser_download | grep linux | cut -d '"' -f 4)
sudo -E curl -L $URL -o /usr/local/bin/kustomize
sudo chmod a+x /usr/local/bin/kustomize

wget -A "openshift-client-linux-4*\.tar\.gz" -r -np -nc -nd -l1 --no-check-certificate -e robots=off https://mirror.openshift.com/pub/openshift-v4/clients/ocp/latest/ -P /tmp/
sudo tar -xvf /tmp/openshift-client-linux-4*.tar.gz -C /usr/local/bin/ oc
rm -f /tmp/openshift-client-linux-4*.tar.gz

