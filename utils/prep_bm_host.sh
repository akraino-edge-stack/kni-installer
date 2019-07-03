#!/bin/bash

###--------------------------------###
### Need interface input from user ###
###--------------------------------###

PROV_INTF=$1
BM_INTF=$2
EXT_INTF=$3
# TODO: Check args

###----------------------------------###
### Configure provisioning interface ###
###----------------------------------###

cat <<EOF > /etc/sysconfig/network-scripts/ifcfg-eno2
TYPE=Ethernet
PROXY_METHOD=none
BROWSER_ONLY=no
BOOTPROTO=static
DEFROUTE=yes
IPV4_FAILURE_FATAL=no
NAME=$PROV_INTF
DEVICE=$PROV_INTF
ONBOOT=yes
IPADDR=172.22.0.10
NETMASK=255.255.255.0
EOF

ifdown $PROV_INTF
ifup $PROV_INTF

###-------------------------------###
### Configure baremetal interface ###
###-------------------------------###

cat <<EOF > /etc/sysconfig/network-scripts/ifcfg-ens1f0
TYPE=Ethernet
NM_CONTROLLED=no
PROXY_METHOD=none
BROWSER_ONLY=no
BOOTPROTO=static
DEFROUTE=no
IPV4_FAILURE_FATAL=no
IPV6INIT=yes
IPV6_AUTOCONF=yes
IPV6_DEFROUTE=yes
IPV6_FAILURE_FATAL=no
IPV6_ADDR_GEN_MODE=stable-privacy
NAME=$BM_INTF
DEVICE=$BM_INTF
IPADDR=192.168.111.1
NETMASK=255.255.255.0
ONBOOT=yes
EOF

ifdown $BM_INTF
ifup $BM_INTF

###-----------------------------###
### Create required directories ###
###-----------------------------###

mkdir -p ~/dev/test1
mkdir -p ~/dev/upi-dnsmasq/$PROV_INTF
mkdip ~/dev/upi-dnsmasq/$BM_INTF
mkdir ~/dev/scripts
sudo mkdir /etc/matchbox
mkdir ~/.matchbox
sudo mkdir -p /var/lib/matchbox/assets
sudo mkdir /etc/coredns

###--------------------------------------------------###
### Configure iptables to allow for external traffic ###
###--------------------------------------------------###

cat <<EOF > ~/dev/scripts/iptables.sh
#!/bin/bash

ins_del_rule()
{
    operation=$1
    table=$2
    rule=$3
   
    if [ "$operation" == "INSERT" ]; then
        if ! sudo iptables -t "$table" -C $rule > /dev/null 2>&1; then
            sudo iptables -t "$table" -I $rule
        fi
    elif [ "$operation" == "DELETE" ]; then
        sudo iptables -t "$table" -D $rule
    else
        echo "${FUNCNAME[0]}: Invalid operation: $operation"
        exit 1
    fi
}

    #allow DNS/DHCP traffic to dnsmasq
    ins_del_rule "INSERT" "filter" "INPUT -i $BM_INTF -p udp -m udp --dport 67 -j ACCEPT"
    ins_del_rule "INSERT" "filter" "INPUT -i $BM_INTF -p udp -m udp --dport 53 -j ACCEPT"
   
    #enable routing from cluster network to external
    ins_del_rule "INSERT" "nat" "POSTROUTING -o $EXT_INTF -j MASQUERADE"
    ins_del_rule "INSERT" "filter" "FORWARD -i $PROV_INTF -o $EXT_INTF -j ACCEPT"
    ins_del_rule "INSERT" "filter" "FORWARD -o $PROV_INTF -i $EXT_INTF -m state --state RELATED,ESTABLISHED -j ACCEPT"
EOF

pushd ~/dev/scripts
chmod 755 iptables.sh
./iptables.sh
popd


###-------------###
### Install Git ###
###-------------###

# TODO: How without using yum?

###----------------###
### Install podman ###
###----------------###

# TODO: How without using yum?

###----------------###
### Install Golang ###
###----------------###

# TODO

###-----------------###
### Set up tftpboot ###
###-----------------###

mkdir -p /var/lib/tftpboot
pushd /var/lib/tftpboot
curl -O http://boot.ipxe.org/ipxe.efi
curl -O http://boot.ipxe.org/undionly.kpxe
popd

###------------------------------###
### Create HAProxy configuration ###
###------------------------------###

cat <<EOF > /etc/haproxy/haproxy.cfg
#---------------------------------------------------------------------
# Global settings
#---------------------------------------------------------------------
global
    log         127.0.0.1 local2

    chroot      /var/lib/haproxy
    pidfile     /var/run/haproxy.pid
    maxconn     4000
    user        haproxy
    group       haproxy
    daemon

    # turn on stats unix socket
    stats socket /var/lib/haproxy/stats

#---------------------------------------------------------------------
# common defaults that all the 'listen' and 'backend' sections will
# use if not designated in their block
#---------------------------------------------------------------------
defaults
    mode                    http
    log                     global
    option                  httplog
    option                  dontlognull
    option forwardfor       except 127.0.0.0/8
    option                  redispatch
    retries                 3
    timeout http-request    10s
    timeout queue           1m
    timeout connect         10s
    timeout client          1m
    timeout server          1m
    timeout http-keep-alive 10s
    timeout check           10s
    maxconn                 3000

frontend kubeapi
    mode tcp
    bind *:6443
    option tcplog
    default_backend kubeapi-main

frontend mcs
    bind *:22623
    default_backend mcs-main
    mode tcp
    option tcplog

frontend http
    bind *:80
    mode tcp
    default_backend http-main
    option tcplog

frontend https
    bind *:443
    mode tcp
    default_backend https-main
    option tcplog

backend kubeapi-main
    balance source
    mode tcp
    server bootstrap 192.168.111.10:6443 check
    server master-0  192.168.111.11:6443 check

backend mcs-main
    balance source
    mode tcp
    server bootstrap 192.168.111.10:22623 check
    server master-0  192.168.111.11:22623 check

backend http-main
    balance source
    mode tcp
    server worker-0  192.168.111.50:80 check

backend https-main
    balance source
    mode tcp
    server worker-0  192.168.111.50:443 check
EOF

###-------------------------###
### Start HAProxy container ###
###-------------------------###

# TODO
# Something like this?  Needs to be able to use haproxy:haproxy user and group
# podman run -d --net=host -v /etc/haproxy:/etc/haproxy:Z,ro docker.io/haproxy -f /etc/haproxy/haproxy.cfg -Ds 

###-------------------------------------------###
### Create provisioning dnsmasq configuration ###
###-------------------------------------------###

cat <<EOF > ~/dev/upi-dnsmasq/$PROV_INTF/dnsmasq.conf
port=0 # do not activate nameserver
interface=$PROV_INTF
bind-interfaces

dhcp-range=172.22.0.11,172.22.0.30,30m

# do not send default route
dhcp-option=3
dhcp-option=6

# Legacy PXE
dhcp-match=set:bios,option:client-arch,0
dhcp-boot=tag:bios,undionly.kpxe

# UEFI
dhcp-match=set:efi32,option:client-arch,6
dhcp-boot=tag:efi32,ipxe.efi
dhcp-match=set:efibc,option:client-arch,7
dhcp-boot=tag:efibc,ipxe.efi
dhcp-match=set:efi64,option:client-arch,9
dhcp-boot=tag:efi64,ipxe.efi

# verbose
log-queries
log-dhcp

dhcp-leasefile=/var/run/dnsmasq/$PROV_INTF.leasefile
log-facility=/var/run/dnsmasq/$PROV_INTF.log

# iPXE - chainload to matchbox ipxe boot script
dhcp-userclass=set:ipxe,iPXE
dhcp-boot=tag:ipxe,http://172.22.0.10:8080/boot.ipxe

# Enable dnsmasq's built-in TFTP server
enable-tftp

# Set the root directory for files available via FTP.
tftp-root=/var/lib/tftpboot

tftp-no-blocksize

dhcp-boot=pxelinux.0

conf-dir=/etc/dnsmasq.d,.rpmnew,.rpmsave,.rpmorig

EOF

###----------------------------------------###
### Create baremetal dnsmasq configuration ###
###----------------------------------------###

cat <<EOF > ~/dev/upi-dnsmasq/$BM_INTF/dnsmasq.conf
port=0
interface=$BM_INTF
bind-interfaces

strict-order
pid-file=/var/run/dnsmasq/$BM_INTF.pid
except-interface=lo

dhcp-range=192.168.111.10,192.168.111.60,30m
#default gateway
dhcp-option=3,192.168.111.1
#dns server
dhcp-option=6,192.168.111.1

log-queries
log-dhcp

dhcp-no-override
dhcp-authoritative
dhcp-hostsfile=/root/dev/upi-dnsmasq/$BM_INTF/$BM_INTF.hostsfile

dhcp-leasefile=/var/run/dnsmasq/$BM_INTF.leasefile
log-facility=/var/run/dnsmasq/$BM_INTF.log

EOF

###--------------------------------------###
### Start provisioning dnsmasq container ###
###--------------------------------------###

# TODO

###-----------------------------------###
### Start baremetal dnsmasq container ###
###-----------------------------------###

# TODO

###--------------------###
### Configure matchbox ###
###--------------------###

git clone https://github.com/poseidon/matchbox.git
pushd matchbox/scripts/tls
export SAN=IP.1:172.22.0.10
./cert-gen
sudo cp ca.crt server.crt server.key /etc/matchbox
cp ca.crt client.crt client.key ~/.matchbox 
popd

###--------------------------###
### Start matchbox container ###
###--------------------------###

podman run -d --net=host -v /var/lib/matchbox:/var/lib/matchbox:Z -v /etc/matchbox:/etc/matchbox:Z,ro quay.io/poseidon/matchbox:latest -address=0.0.0.0:8080 -rpc-address=0.0.0.0:8081 -log-level=debug

###----------------------------------------###
### Configure coredns Corefile and db file ###
###----------------------------------------###

cat <<EOF > /etc/coredns/Corefile
.:53 {
    log
    errors
    forward . 10.11.5.19
}

tt.testing:53 {
    log
    errors
    file /etc/coredns/db.tt.testing
    debug
}

EOF

cat <<EOF > /etc/coredns/db.tt.testing
$ORIGIN tt.testing.
$TTL 10800      ; 3 hours
@       3600 IN SOA sns.dns.icann.org. noc.dns.icann.org. (
                                2019010101 ; serial
                                7200       ; refresh (2 hours)
                                3600       ; retry (1 hour)
                                1209600    ; expire (2 weeks)
                                3600       ; minimum (1 hour)
                                

_etcd-server-ssl._tcp.test1.tt.testing. 8640 IN    SRV 0 10 2380 etcd-0.test1.tt.testing.

api.test1.tt.testing.                        A 192.168.111.1
api-int.test1.tt.testing.                    A 192.168.111.1
test1-master-0.tt.testing.                   A 192.168.111.11
test1-worker-0.tt.testing.                   A 192.168.111.50
test1-bootstrap.tt.testing.                  A 192.168.111.10
etcd-0.test1.tt.testing.                     IN  CNAME test1-master-0.tt.testing.

$ORIGIN apps.test1.tt.testing.
*                                                    A                10.19.110.5
EOF

###-------------------------###
### Start coredns container ###
###-------------------------###

podman run -d --expose=53 --expose=53/udp -p 192.168.111.1:53:53 -p 192.168.111.1:53:53/udp -v /etc/coredns:/etc/coredns:z --name coredns coredns/coredns:latest -conf /etc/coredns/Corefile

###----------------------------###
### Prepare OpenShift binaries ###
###----------------------------###

pushd /tmp
curl -O https://mirror.openshift.com/pub/openshift-v4/clients/ocp/latest/openshift-install-linux-4.1.3.tar.gz
tar xvf openshift-install-linux-4.1.2.tar.gz
sudo mv openshift-install /usr/local/bin/
curl -O https://mirror.openshift.com/pub/openshift-v4/clients/ocp/latest/openshift-client-linux-4.1.3.tar.gz
tar xvf openshift-client-linux-4.1.2.tar.gz
sudo mv oc /usr/local/bin/

###-------------------###
### Prepare terraform ###
###-------------------###

# TODO: How to acquire unzip and impitool without using yum?
curl -O https://releases.hashicorp.com/terraform/0.12.2/terraform_0.12.2_linux_amd64.zip
unzip terraform_0.12.2_linux_amd64.zip
sudo mv terraform /usr/bin/.
git clone https://github.com/poseidon/terraform-provider-matchbox.git
cd terraform-provider-matchbox
go build
mkdir -p ~/.terraform.d/plugins
cp terraform-provider-matchbox ~/.terraform.d/plugins/.
popd

### >>>>>>>>>>>>>>>>>>>>>>>>>> Is what follows needed in host prep script? <<<<<<<<<<<<<<<<<<<<<<<< ###

###-------------------###
### Clone UPI-RT repo ###
###-------------------###

pushd ~/dev
git clone https://github.com/redhat-nfvpe/upi-rt.git
popd

###----------------------###
### Initialize terraform ###
###----------------------###

pushd ~/dev/upi-rt/terraform
terraform init
popd

###----------------------------###
### Create install-config.yaml ###
###----------------------------###

# TODO: How to handle pull secret?
PUB_KEY=`cat ~/.ssh/id_rsa.pub`
cat <<EOF > ~/dev/test1/install-config.yaml
apiVersion: v1
baseDomain: tt.testing
compute:
 - name: worker
   replicas: 1
controlPlane:
   name: master
   platform: {}
   replicas: 1
metadata:
   name: test1
networking:
  clusterNetworks:
  - cidr: 10.128.0.0/14
    hostPrefix: 23
  networkType: OpenShiftSDN
  serviceNetwork:
  - 172.30.0.0/16
platform:
   none: {}
pullSecret: ‘CHANGEME’
sshKey: |
   $PUB_KEY
EOF

###------------------###
### Create ignitions ###
###------------------###

openshift-install create ignition-configs --dir=/$HOME/dev/test1

###-----------------###
### Apply terraform ###
###-----------------###

pushd ~/dev/upi-rt/terraform/cluster
terraform apply --auto-approve
popd

###-----------------------------------###
### Wait for completion of deployment ###
###-----------------------------------###

openshift-install --dir=/$HOME/dev/test1 wait-for bootstrap-complete --log-level debug
